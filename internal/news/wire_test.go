package news

import (
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/go-resty/resty/v2"
	"github.com/redis/go-redis/v9"
)

func testRedisCmdable() redis.Cmdable {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
}

func TestNewServiceFromConfig_nilInputs(t *testing.T) {
	if NewServiceFromConfig(nil, nil, nil) != nil {
		t.Fatal("expected nil")
	}
	cfg := &config.Config{NewsProvider: "thenewsapi", TheNewsAPIToken: "t"}
	if NewServiceFromConfig(cfg, nil, resty.New()) != nil {
		t.Fatal("expected nil without redis")
	}
	if NewServiceFromConfig(cfg, testRedisCmdable(), nil) != nil {
		t.Fatal("expected nil without http client")
	}
}

func TestNewServiceFromConfig_requiresToken(t *testing.T) {
	cfg := &config.Config{NewsProvider: "thenewsapi"}
	if NewServiceFromConfig(cfg, testRedisCmdable(), resty.New()) != nil {
		t.Fatal("expected nil without API token")
	}
}

func TestNewServiceFromConfig_requiresTheNewsAPI(t *testing.T) {
	cfg := &config.Config{
		NewsProvider:    "other",
		TheNewsAPIToken: "x",
	}
	if NewServiceFromConfig(cfg, testRedisCmdable(), resty.New()) != nil {
		t.Fatal("expected nil for unsupported provider")
	}
}

func TestNewServiceFromConfig_ok(t *testing.T) {
	cfg := &config.Config{
		NewsProvider:    "thenewsapi",
		TheNewsAPIToken: "secret",
	}
	s := NewServiceFromConfig(cfg, testRedisCmdable(), resty.New())
	if s == nil || s.Provider == nil || s.Cache == nil {
		t.Fatal("expected full service")
	}
}
