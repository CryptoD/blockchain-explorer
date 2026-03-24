package pricing

import (
	"context"
	"errors"
)

// MockClient implements Client for tests without calling CoinGecko or other HTTP APIs.
type MockClient struct {
	GetMultiCurrencyRatesFunc   func(ctx context.Context) (map[string]interface{}, error)
	GetMultiCurrencyRatesInFunc func(ctx context.Context, currencies []string) (map[string]interface{}, error)
	GetBTCUSDFunc               func(ctx context.Context) (float64, error)
}

// GetMultiCurrencyRates implements Client.
func (m *MockClient) GetMultiCurrencyRates(ctx context.Context) (map[string]interface{}, error) {
	if m.GetMultiCurrencyRatesFunc != nil {
		return m.GetMultiCurrencyRatesFunc(ctx)
	}
	return m.GetMultiCurrencyRatesIn(ctx, nil)
}

// GetMultiCurrencyRatesIn implements Client.
func (m *MockClient) GetMultiCurrencyRatesIn(ctx context.Context, currencies []string) (map[string]interface{}, error) {
	if m.GetMultiCurrencyRatesInFunc != nil {
		return m.GetMultiCurrencyRatesInFunc(ctx, currencies)
	}
	return nil, errors.New("pricing.MockClient: GetMultiCurrencyRatesInFunc not set")
}

// GetBTCUSD implements Client.
func (m *MockClient) GetBTCUSD(ctx context.Context) (float64, error) {
	if m.GetBTCUSDFunc != nil {
		return m.GetBTCUSDFunc(ctx)
	}
	return 0, errors.New("pricing.MockClient: GetBTCUSDFunc not set")
}

// MockCryptoPriceFetcher implements CryptoPriceFetcher for tests.
type MockCryptoPriceFetcher struct {
	GetCryptoPriceInFiatFunc func(ctx context.Context, coinID, fiat string) (float64, bool)
}

// GetCryptoPriceInFiat implements CryptoPriceFetcher.
func (m *MockCryptoPriceFetcher) GetCryptoPriceInFiat(ctx context.Context, coinID, fiat string) (float64, bool) {
	if m == nil || m.GetCryptoPriceInFiatFunc == nil {
		return 0, false
	}
	return m.GetCryptoPriceInFiatFunc(ctx, coinID, fiat)
}
