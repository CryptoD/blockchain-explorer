package server

import (
	"net"
	"net/http"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/go-resty/resty/v2"
	"github.com/redis/go-redis/v9"
)

// newRestyClientForConfig builds the shared Resty client used for blockchain RPC, pricing, and news.
// Transport limits and request timeout align with config (defaults match JSON-RPC deadline in getusdperfiat.go).
func newRestyClientForConfig(cfg *config.Config) *resty.Client {
	if cfg == nil {
		return resty.New().SetTimeout(30 * time.Second).SetRetryCount(3)
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxConnsPerHost = cfg.OutboundHTTPMaxConnsPerHost
	tr.MaxIdleConns = cfg.OutboundHTTPMaxIdleConns
	tr.MaxIdleConnsPerHost = cfg.OutboundHTTPMaxIdleConnsPerHost
	tr.IdleConnTimeout = time.Duration(cfg.OutboundHTTPIdleConnTimeoutSeconds) * time.Second
	// Bound dial + TLS handshake so slow upstreams cannot tie up goroutines indefinitely.
	tr.DialContext = (&net.Dialer{
		Timeout:   time.Duration(cfg.OutboundHTTPDialTimeoutSeconds) * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.ResponseHeaderTimeout = time.Duration(cfg.OutboundHTTPResponseHeaderTimeoutSeconds) * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second

	hc := &http.Client{
		Timeout:   time.Duration(cfg.OutboundHTTPTimeoutSeconds) * time.Second,
		Transport: tr,
	}
	return resty.NewWithClient(hc).
		SetRetryCount(3).
		SetHeader("User-Agent", "blockchain-explorer/1.0")
}

// redisOptionsFromConfig returns go-redis options for the application Redis client.
func redisOptionsFromConfig(cfg *config.Config) *redis.Options {
	if cfg == nil {
		return &redis.Options{Addr: "localhost:6379"}
	}
	o := &redis.Options{
		Addr: cfg.RedisAddr(),
		// Respect context deadlines (e.g. request-scoped timeouts).
		ContextTimeoutEnabled: true,
		DialTimeout:           time.Duration(cfg.RedisDialTimeoutSeconds) * time.Second,
		ReadTimeout:           time.Duration(cfg.RedisReadTimeoutSeconds) * time.Second,
		WriteTimeout:          time.Duration(cfg.RedisWriteTimeoutSeconds) * time.Second,
		MinIdleConns:          cfg.RedisMinIdleConns,
	}
	if cfg.RedisConnMaxIdleSeconds > 0 {
		o.ConnMaxIdleTime = time.Duration(cfg.RedisConnMaxIdleSeconds) * time.Second
	}
	if cfg.RedisPoolSize > 0 {
		o.PoolSize = cfg.RedisPoolSize
	}
	if cfg.RedisMaxActiveConns > 0 {
		o.MaxActiveConns = cfg.RedisMaxActiveConns
	}
	if cfg.RedisPoolTimeoutSeconds > 0 {
		o.PoolTimeout = time.Duration(cfg.RedisPoolTimeoutSeconds) * time.Second
	}
	return o
}
