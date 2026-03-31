package pricing

import (
	"context"
	"strings"
	"sync"
	"time"
)

// CachingClient wraps Client with an in-memory TTL cache for successful rate responses.
type CachingClient struct {
	underlying Client
	ttl        time.Duration

	mu          sync.Mutex
	ratesAt     time.Time
	ratesCached map[string]interface{}
	ratesKey    string

	btcUSDAt     time.Time
	btcUSDCached float64
}

// NewCachingClient returns a Client that caches GetMultiCurrencyRatesIn and GetBTCUSD results.
// ttl must be positive; otherwise returns underlying unchanged (no-op wrapper not used).
func NewCachingClient(underlying Client, ttl time.Duration) Client {
	if underlying == nil || ttl <= 0 {
		return underlying
	}
	return &CachingClient{underlying: underlying, ttl: ttl}
}

func cacheKeyForCurrencies(currencies []string) string {
	vs := NormalizeAndFilterFiatCurrencies(currencies)
	if len(vs) == 0 {
		vs = append([]string(nil), DefaultFiatCurrencies...)
	}
	// stable key for same set
	return strings.Join(vs, ",")
}

func (c *CachingClient) GetMultiCurrencyRates(ctx context.Context) (map[string]interface{}, error) {
	return c.GetMultiCurrencyRatesIn(ctx, nil)
}

func (c *CachingClient) GetMultiCurrencyRatesIn(ctx context.Context, currencies []string) (map[string]interface{}, error) {
	key := cacheKeyForCurrencies(currencies)
	now := time.Now()

	c.mu.Lock()
	if c.ratesCached != nil && c.ratesKey == key && !now.After(c.ratesAt.Add(c.ttl)) {
		out := shallowCopyRatesMap(c.ratesCached)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	rates, err := c.underlying.GetMultiCurrencyRatesIn(ctx, currencies)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.ratesCached = rates
	c.ratesKey = key
	c.ratesAt = now
	c.mu.Unlock()
	return rates, nil
}

func shallowCopyRatesMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *CachingClient) GetBTCUSD(ctx context.Context) (float64, error) {
	now := time.Now()
	c.mu.Lock()
	if !now.After(c.btcUSDAt.Add(c.ttl)) && c.btcUSDAt != (time.Time{}) {
		v := c.btcUSDCached
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	v, err := c.underlying.GetBTCUSD(ctx)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	c.btcUSDCached = v
	c.btcUSDAt = now
	c.mu.Unlock()
	return v, nil
}

// CachingCryptoFetcher wraps CryptoPriceFetcher with per-(coinID,fiat) TTL caching.
type CachingCryptoFetcher struct {
	underlying CryptoPriceFetcher
	ttl        time.Duration

	mu    sync.Mutex
	entry map[string]cachedCrypto
}

type cachedCrypto struct {
	at    time.Time
	price float64
	ok    bool
}

// NewCachingCryptoFetcher returns a CryptoPriceFetcher that memoizes successful lookups.
func NewCachingCryptoFetcher(underlying CryptoPriceFetcher, ttl time.Duration) CryptoPriceFetcher {
	if underlying == nil || ttl <= 0 {
		return underlying
	}
	return &CachingCryptoFetcher{
		underlying: underlying,
		ttl:        ttl,
		entry:      make(map[string]cachedCrypto),
	}
}

func cryptoCacheKey(coinID, fiat string) string {
	return strings.ToLower(strings.TrimSpace(coinID)) + "|" + strings.ToLower(strings.TrimSpace(fiat))
}

// GetCryptoPriceInFiat implements CryptoPriceFetcher.
func (c *CachingCryptoFetcher) GetCryptoPriceInFiat(ctx context.Context, coinID, fiat string) (float64, bool) {
	key := cryptoCacheKey(coinID, fiat)
	now := time.Now()

	c.mu.Lock()
	if e, ok := c.entry[key]; ok && !now.After(e.at.Add(c.ttl)) {
		c.mu.Unlock()
		return e.price, e.ok
	}
	c.mu.Unlock()

	price, ok := c.underlying.GetCryptoPriceInFiat(ctx, coinID, fiat)

	c.mu.Lock()
	c.entry[key] = cachedCrypto{at: now, price: price, ok: ok}
	c.mu.Unlock()
	return price, ok
}

// Compile-time check (underlying must implement CryptoPriceFetcher).
var _ CryptoPriceFetcher = (*CachingCryptoFetcher)(nil)

// Compile-time check.
var _ Client = (*CachingClient)(nil)
