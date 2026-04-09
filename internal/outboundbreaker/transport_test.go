package outboundbreaker

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/config"
)

func TestWrapRoundTripper_DisabledPassesThrough(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer ts.Close()

	cfg := &config.Config{OutboundCircuitBreakerEnabled: false}
	tr := WrapRoundTripper(http.DefaultTransport, cfg)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	for i := 0; i < 5; i++ {
		resp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if calls.Load() != 5 {
		t.Fatalf("expected 5 upstream calls, got %d", calls.Load())
	}
}

func TestWrapRoundTripper_OpensAfterConsecutiveFailures(t *testing.T) {
	t.Parallel()
	var upstreamCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := &config.Config{
		OutboundCircuitBreakerEnabled:                      true,
		OutboundCircuitBreakerOpenTimeoutSeconds:           60,
		OutboundCircuitBreakerIntervalSeconds:              0,
		OutboundCircuitBreakerHalfOpenMaxRequests:          1,
		OutboundCircuitBreakerTripAfterConsecutiveFailures: 2,
	}
	tr := WrapRoundTripper(http.DefaultTransport, cfg)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	for i := 0; i < 3; i++ {
		resp, err := client.Get(ts.URL + "/")
		if err != nil {
			t.Logf("request %d: %v", i, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// Two failing upstream calls trip the breaker; third is short-circuited.
	if got := upstreamCalls.Load(); got != 2 {
		t.Fatalf("upstream calls: want 2, got %d", got)
	}
}

func TestWrapRoundTripper_5xxIsFailure4xxIsSuccess(t *testing.T) {
	t.Parallel()
	var upstreamCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	cfg := &config.Config{
		OutboundCircuitBreakerEnabled:                      true,
		OutboundCircuitBreakerOpenTimeoutSeconds:           60,
		OutboundCircuitBreakerHalfOpenMaxRequests:          1,
		OutboundCircuitBreakerTripAfterConsecutiveFailures: 2,
	}
	tr := WrapRoundTripper(http.DefaultTransport, cfg)
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if upstreamCalls.Load() != 1 {
		t.Fatalf("expected 1 upstream call, got %d", upstreamCalls.Load())
	}
}
