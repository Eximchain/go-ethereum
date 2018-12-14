package private

import (
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
}

func FromCommandLineEnvironmentOrNil(name string) PrivateTransactionManager {
	cfgPath := CliCfgPath
	if cfgPath == "" {
		cfgPath = os.Getenv(name)
	}
	if cfgPath == "" {
		return nil
	}
	return constellation.MustNew(cfgPath)
}

var P = FromCommandLineEnvironmentOrNil("PRIVATE_CONFIG")

func RegeneratePrivateConfig() {
	if P == nil {
		P = FromCommandLineEnvironmentOrNil("PRIVATE_CONFIG")
	}
}
