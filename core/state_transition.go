// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"errors"
	"math"
	"math/big"

	"github.com/eximchain/go-ethereum/common"
	"github.com/eximchain/go-ethereum/core/vm"
	"github.com/eximchain/go-ethereum/log"
	"github.com/eximchain/go-ethereum/params"
	"github.com/eximchain/go-ethereum/private"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==
  4a) Attempt to run transaction data
  4b) If valid, use result as code for the new state object
== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// PrivateMessage implements a private message
type PrivateMessage interface {
	Message
	IsPrivate() bool
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	log.Info("IntrinsicGas start with args", "data", data, "contractCreation", contractCreation, "homestead", homestead)
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	log.Info("IntrinsicGas starting gas for raw transaction", "gas", gas)
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		if (math.MaxUint64-gas)/params.TxDataNonZeroGas < nz {
			log.Warn("IntrinsicGas ErrOutOfGas")
			return 0, vm.ErrOutOfGas
		}
		gas += nz * params.TxDataNonZeroGas
		log.Info("IntrinsicGas after data nonzero gas", "gas", gas)

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			log.Warn("IntrinsicGas ErrOutOfGas")
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
		log.Info("IntrinsicGas after data zero gas", "gas", gas)
	}
	log.Info("IntrinsicGas before return", "gas", gas)
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.PublicState(),
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) ([]byte, uint64, bool, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

func (st *StateTransition) from() vm.AccountRef {
	f := st.msg.From()
	if !st.state.Exist(f) {
		st.state.CreateAccount(f)
	}
	return vm.AccountRef(f)
}

// to returns the recipient of the message.
func (st *StateTransition) to() vm.AccountRef {
	if st.msg == nil {
		return vm.AccountRef{}
	}
	to := st.msg.To()
	if to == nil {
		return vm.AccountRef{} // contract creation
	}

	reference := vm.AccountRef(*to)
	if !st.state.Exist(*to) {
		st.state.CreateAccount(*to)
	}
	return reference
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	if st.state.GetBalance(st.msg.From()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()
	st.state.SubBalance(st.msg.From(), mgval)
	return nil
}

func (st *StateTransition) preCheck() error {
	msg := st.msg
	sender := st.from()

	// Make sure this transaction's nonce is correct
	if msg.CheckNonce() {
		nonce := st.state.GetNonce(sender.Address())
		if nonce < msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > msg.Nonce() {
			return ErrNonceTooLow
		}
	}
	return st.buyGas()
}

// DONE: logic for private transaction state transitions (nonce changes and data fetched from encrypted backend)
// TransitionDb will transition the state by applying the current message and
// returning the result including the used gas. It returns an error if failed.
// An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, usedGas uint64, failed bool, err error) {
	if err = st.preCheck(); err != nil {
		return
	}
	msg := st.msg
	sender := st.from()

	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	contractCreation := msg.To() == nil
	privacyProtocol := true

	var data []byte
	isPrivate := false
	publicState := st.state
	//DONE: implement PrivateMessage Struct to wrap Message interface
	if msg, ok := msg.(PrivateMessage); ok && privacyProtocol && msg.IsPrivate() {
		isPrivate = true
		//DONE: actually fetch the private transaction from constellation
		//data, err = private.P.Receive(st.data)
		data, err = private.P.Receive(st.data)
		// Increment the public account nonce if:
		// 1. Tx is private and *not* a participant of the group and either call or create
		// 2. Tx is private we are part of the group and is a call
		// NOTE: smells in original why not pass back error?
		if err != nil || !contractCreation {
			publicState.SetNonce(sender.Address(), publicState.GetNonce(sender.Address())+1)
		}
		if err != nil {
			return nil, 0, false, err
		}
	} else {
		data = st.data
	}

	// Pay intrinsic gas
	gas, err := IntrinsicGas(data, contractCreation, homestead)
	if err != nil {
		log.Warn("TransitionDb: IntrinsicGas Error", "err", err)
		return nil, 0, false, err
	}
	log.Info("TransitionDb: IntrinsicGas Paid")
	if err = st.useGas(gas); err != nil {
		log.Warn("TransitionDb: st.useGas Error", "err", err, "gas", gas)
		return nil, 0, false, err
	}

	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefor
		// not assigned to err, except for insufficient balance
		// error.
		vmerr error

		contractAddr common.Address
	)
	if contractCreation {
		if isPrivate {
			log.Warn("TransitionDb: Creating private contract in EVM", "sender", sender, "data", data, "st.gas", st.gas, "st.value", st.value)
		} else {
			log.Warn("TransitionDb: Creating public contract in EVM", "sender", sender, "data", data, "st.gas", st.gas, "st.value", st.value)
		}
		ret, contractAddr, st.gas, vmerr = evm.Create(sender, data, st.gas, st.value)
		log.Warn("TransitionDb: evm.Create call complete", "ret", ret, "contractAddr", contractAddr, "st.gas", st.gas, "vmerr", vmerr)
	} else {
		// DONE: Increment the account nonce only if the transaction isn't private.
		// If the transaction is private it has already been incremented on
		// the public state.
		if !isPrivate {
			publicState.SetNonce(sender.Address(), publicState.GetNonce(sender.Address())+1)
		}
		// NOTE: prevent public state being created when a private transaction
		// call is initiated use the msg's address rather than using the to method
		// on the state transition object.

		var to common.Address
		to = *st.msg.To()
		//if input is empty for a private smart contract call, return
		if len(data) == 0 && isPrivate {
			log.Warn("TransitionDb: Empty data for private contract call")
			return nil, 0, false, nil
		}
		//DONE: rabbit hole
		log.Warn("TransitionDb: Making EVM call", "sender", sender, "to", to, "data", data, "st.gas", st.gas, "st.value", st.value)
		ret, st.gas, vmerr = evm.Call(sender, to, data, st.gas, st.value)
	}
	if vmerr != nil {
		log.Warn("TransitionDB: VM returned with error", "err", vmerr)
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			log.Warn("TransitionDb: ErrInsufficientBalance")
			return nil, 0, false, vmerr
		}
	}
	log.Warn("TransitionDb: EVM call returned without error", "ret", ret, "st.gas", st.gas)
	st.refundGas()
	log.Warn("TransitionDb: refundGas complete")
	st.state.AddBalance(st.evm.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice))
	log.Warn("TransitionDb: st.state.AddBalance complete")
	if isPrivate {
		log.Warn("TransitionDb: private transaction returning", "ret", ret, "vmerr != nil", vmerr != nil, "err", err)
		return ret, 0, vmerr != nil, err
	}
	log.Warn("TransitionDb: public transaction returning", "ret", ret, "st.gasUsed()", st.gasUsed(), "vmerr != nil", vmerr != nil, "err", err)
	return ret, st.gasUsed(), vmerr != nil, err
}

func (st *StateTransition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / 2
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(st.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
