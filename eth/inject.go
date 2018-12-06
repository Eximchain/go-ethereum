package eth

import (
	"github.com/eximchain/go-ethereum/consensus/ethash"
	"github.com/eximchain/go-ethereum/log"
	"github.com/eximchain/go-ethereum/rpc"
)

//InjectClient makes an rpc client available in consensus engine
//TODO: refactor code to inject private keys here as well
func (s *Ethereum) InjectClient(client *rpc.Client) error {
	if ethash, ok := s.engine.(*ethash.Ethash); ok {
		err := ethash.AuthorizeClient(client)
		if err != nil {
			return err
		}
		log.Info("WEYL CONSENSUS: Add rpc client to ethash consensus engine to call out to smart contract", "injected", err)

	}
	return nil
}
