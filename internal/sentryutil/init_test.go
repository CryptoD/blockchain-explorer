package sentryutil

import (
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
)

func TestInit_NoDSN(t *testing.T) {
	if err := Init(nil); err != nil {
		t.Fatal(err)
	}
	if err := Init(&config.Config{}); err != nil {
		t.Fatal(err)
	}
}
