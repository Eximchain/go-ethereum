package private

import (
	"fmt"
	"os"

	"github.com/jpmorganchase/quorum/private/constellation"
)

type PrivateTransactionManager interface {
	Send(data []byte, from string, to []string) ([]byte, error)
	Receive(data []byte) ([]byte, error)
}

var CliCfgPath = ""

func SetCliCfgPath(cliCfgPath string) {
	CliCfgPath = cliCfgPath
	fmt.Println("Set CliCfgPath:", CliCfgPath)
}

func FromCommandLineEnvironmentOrNil(name string) PrivateTransactionManager {
	cfgPath := CliCfgPath
	fmt.Println("cfgPath 1:", cfgPath)
	if cfgPath == "" {
		cfgPath = os.Getenv(name)
	}
	fmt.Println("cfgPath 2:", cfgPath)
	if cfgPath == "" {
		return nil
	}
	fmt.Println("Loading from cfgPath:", cfgPath)
	return constellation.MustNew(cfgPath)
}

var P = FromCommandLineEnvironmentOrNil("PRIVATE_CONFIG")

func RegeneratePrivateConfig() {
	if P == nil {
		P = FromCommandLineEnvironmentOrNil("PRIVATE_CONFIG")
	}
}
