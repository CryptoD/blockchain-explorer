package pricing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCachingClient_GetMultiCurrencyRatesIn_concurrentColdCacheSingleUnderlyingCall(t *testing.T) {
	var calls int32
	base := &MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 99.0},
			}, nil
		},
		GetBTCUSDFunc: func(ctx context.Context) (float64, error) {
			return 99, nil
		},
	}
	c := NewCachingClient(base, time.Minute).(*CachingClient)
	ctx := context.Background()

	const n = 48
	var wg sync.WaitGroup
	var fail atomic.Int32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			m, err := c.GetMultiCurrencyRatesIn(ctx, []string{"usd"})
			if err != nil {
				fail.Store(1)
				return
			}
			btc, _ := m["bitcoin"].(map[string]interface{})
			if btc == nil || btc["usd"] != 99.0 {
				fail.Store(1)
			}
		}()
	}
	wg.Wait()
	if fail.Load() != 0 {
		t.Fatal("concurrent GetMultiCurrencyRatesIn failures or bad payload")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("stampede: want 1 underlying call, got %d", calls)
	}
}

func TestCachingClient_GetBTCUSD_concurrentColdCacheSingleUnderlyingCall(t *testing.T) {
	var calls int32
	base := &MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			return map[string]interface{}{}, nil
		},
		GetBTCUSDFunc: func(ctx context.Context) (float64, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return 12345, nil
		},
	}
	c := NewCachingClient(base, time.Minute).(*CachingClient)
	ctx := context.Background()

	const n = 48
	var wg sync.WaitGroup
	var fail atomic.Int32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			v, err := c.GetBTCUSD(ctx)
			if err != nil || v != 12345 {
				fail.Store(1)
			}
		}()
	}
	wg.Wait()
	if fail.Load() != 0 {
		t.Fatal("concurrent GetBTCUSD failures")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("stampede: want 1 underlying call, got %d", calls)
	}
}

func TestCachingCryptoFetcher_concurrentColdCacheSingleUnderlyingCall(t *testing.T) {
	var calls int32
	base := &MockCryptoPriceFetcher{
		GetCryptoPriceInFiatFunc: func(ctx context.Context, coinID, fiat string) (float64, bool) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return 77, true
		},
	}
	w := NewCachingCryptoFetcher(base, time.Minute).(*CachingCryptoFetcher)
	ctx := context.Background()

	const n = 48
	var wg sync.WaitGroup
	var fail atomic.Int32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			p, ok := w.GetCryptoPriceInFiat(ctx, "bitcoin", "eur")
			if !ok || p != 77 {
				fail.Store(1)
			}
		}()
	}
	wg.Wait()
	if fail.Load() != 0 {
		t.Fatal("concurrent GetCryptoPriceInFiat failures")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("stampede: want 1 underlying call, got %d", calls)
	}
}
