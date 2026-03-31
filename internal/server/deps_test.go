package server

import (
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/go-resty/resty/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewDependencies_HoldsInterfaceBoundaries(t *testing.T) {
	cfg := &config.Config{
		AppEnv:              "development",
		GetBlockBaseURL:     "https://rpc.example",
		GetBlockAccessToken: "tok",
	}
	cl := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	defer cl.Close()
	st := repos.NewStores(cl)
	httpC := resty.New()
	bc := blockchain.NewGetBlockRPCClient(cfg.GetBlockBaseURL, cfg.GetBlockAccessToken, httpC)
	pc := pricing.NewCoinGeckoClient(httpC)
	ap := &pricing.CompositePricer{Crypto: pc}

	ResetDefaultServices()
	d := NewDependencies(cfg, cl, st, httpC, bc, pc, ap, nil, nil, nil)
	if d.Config != cfg || d.Redis != cl || d.Repos != st || d.HTTP != httpC {
		t.Fatal("unexpected core fields")
	}
	if d.Blockchain != bc || d.Pricing != pc || d.Assets != ap {
		t.Fatal("interface boundaries not wired")
	}
	if d.Explorer == nil || d.Auth == nil {
		t.Fatal("expected default domain services")
	}
}

func TestGetDependencies_ReturnsNilBeforeApply(t *testing.T) {
	saved := appDeps
	appDeps = nil
	defer func() { appDeps = saved }()
	if GetDependencies() != nil {
		t.Fatal("expected nil")
	}
}
