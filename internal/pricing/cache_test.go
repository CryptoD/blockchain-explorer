package pricing

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachingClient_GetMultiCurrencyRatesIn_usesTTL(t *testing.T) {
	var calls int32
	base := &MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			atomic.AddInt32(&calls, 1)
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 1.0},
			}, nil
		},
		GetBTCUSDFunc: func(ctx context.Context) (float64, error) {
			return 1, nil
		},
	}
	c := NewCachingClient(base, 100*time.Millisecond).(*CachingClient)
	ctx := context.Background()
	_, _ = c.GetMultiCurrencyRatesIn(ctx, []string{"usd"})
	_, _ = c.GetMultiCurrencyRatesIn(ctx, []string{"usd"})
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 underlying call, got %d", calls)
	}
	time.Sleep(120 * time.Millisecond)
	_, _ = c.GetMultiCurrencyRatesIn(ctx, []string{"usd"})
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected refresh after TTL, got %d calls", calls)
	}
}

func TestCachingCryptoFetcher(t *testing.T) {
	var calls int32
	base := &MockCryptoPriceFetcher{
		GetCryptoPriceInFiatFunc: func(ctx context.Context, coinID, fiat string) (float64, bool) {
			atomic.AddInt32(&calls, 1)
			return 42, true
		},
	}
	w := NewCachingCryptoFetcher(base, 100*time.Millisecond).(*CachingCryptoFetcher)
	ctx := context.Background()
	v, ok := w.GetCryptoPriceInFiat(ctx, "bitcoin", "usd")
	if !ok || v != 42 {
		t.Fatalf("first: %v %v", v, ok)
	}
	v2, ok2 := w.GetCryptoPriceInFiat(ctx, "bitcoin", "usd")
	if !ok2 || v2 != 42 {
		t.Fatalf("second: %v %v", v2, ok2)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
