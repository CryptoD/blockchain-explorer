// Package outboundbreaker wraps http.RoundTripper with per-host circuit breakers (sony/gobreaker)
// for shared outbound traffic: GetBlock JSON-RPC, CoinGecko, news HTTP APIs.
package outboundbreaker

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/sony/gobreaker"
)

// WrapRoundTripper returns base unchanged when cfg is nil or outbound circuit breaking is disabled.
func WrapRoundTripper(base http.RoundTripper, cfg *config.Config) http.RoundTripper {
	if base == nil || cfg == nil || !cfg.OutboundCircuitBreakerEnabled {
		return base
	}
	return &hostTransport{base: base, cfg: cfg}
}

type hostTransport struct {
	base http.RoundTripper
	cfg  *config.Config

	breakers sync.Map // host string -> *gobreaker.CircuitBreaker
}

func (t *hostTransport) breaker(host string) *gobreaker.CircuitBreaker {
	if v, ok := t.breakers.Load(host); ok {
		return v.(*gobreaker.CircuitBreaker)
	}
	cb := gobreaker.NewCircuitBreaker(settingsForHost(host, t.cfg))
	if v, loaded := t.breakers.LoadOrStore(host, cb); loaded {
		return v.(*gobreaker.CircuitBreaker)
	}
	return cb
}

func settingsForHost(host string, cfg *config.Config) gobreaker.Settings {
	open := time.Duration(cfg.OutboundCircuitBreakerOpenTimeoutSeconds) * time.Second
	interval := time.Duration(cfg.OutboundCircuitBreakerIntervalSeconds) * time.Second
	maxHalf := cfg.OutboundCircuitBreakerHalfOpenMaxRequests
	if maxHalf < 1 {
		maxHalf = 1
	}
	s := gobreaker.Settings{
		Name:        host,
		MaxRequests: uint32(maxHalf),
		Interval:    interval,
		Timeout:     open,
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			metrics.RecordOutboundCircuitBreakerTransition(
				metricsSanitizeHost(name),
				from.String(),
				to.String(),
			)
		},
	}
	if thr := cfg.OutboundCircuitBreakerTripAfterConsecutiveFailures; thr > 0 {
		s.ReadyToTrip = func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(thr)
		}
	}
	return s
}

// RoundTrip enforces one circuit breaker per req.URL.Host (scheme ignored; TLS is transparent here).
func (t *hostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return t.base.RoundTrip(req)
	}
	host := strings.TrimSpace(req.URL.Host)
	if host == "" {
		return t.base.RoundTrip(req)
	}

	cb := t.breaker(host)
	raw, err := cb.Execute(func() (interface{}, error) {
		resp, rerr := t.base.RoundTrip(req)
		if rerr != nil {
			return nil, rerr
		}
		if resp == nil {
			return nil, fmt.Errorf("outbound: nil response")
		}
		if resp.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("upstream HTTP %d", resp.StatusCode)
		}
		return resp, nil
	})
	if err != nil {
		h := metricsSanitizeHost(host)
		switch {
		case errors.Is(err, gobreaker.ErrOpenState):
			metrics.RecordOutboundCircuitBreakerReject(h, "open")
		case errors.Is(err, gobreaker.ErrTooManyRequests):
			metrics.RecordOutboundCircuitBreakerReject(h, "half_open")
		}
		return nil, fmt.Errorf("outbound circuit breaker %q: %w", host, err)
	}
	return raw.(*http.Response), nil
}

func metricsSanitizeHost(host string) string {
	host = strings.TrimSpace(host)
	if len(host) > 128 {
		host = host[:128]
	}
	host = strings.ReplaceAll(host, "\n", "_")
	return host
}
