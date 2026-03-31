package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// CoinGeckoClient is a concrete implementation of Client using the CoinGecko HTTP API only.
type CoinGeckoClient struct {
	HTTPClient *resty.Client
}

// NewCoinGeckoClient constructs a CoinGeckoClient, defaulting to a sane Resty
// client configuration if httpClient is nil.
func NewCoinGeckoClient(httpClient *resty.Client) *CoinGeckoClient {
	if httpClient == nil {
		httpClient = resty.New().
			SetTimeout(10 * time.Second).
			SetRetryCount(3)
	}
	return &CoinGeckoClient{HTTPClient: httpClient}
}

func (c *CoinGeckoClient) GetMultiCurrencyRates(ctx context.Context) (map[string]interface{}, error) {
	return c.GetMultiCurrencyRatesIn(ctx, nil)
}

func (c *CoinGeckoClient) GetMultiCurrencyRatesIn(ctx context.Context, currencies []string) (map[string]interface{}, error) {
	vs := NormalizeAndFilterFiatCurrencies(currencies)
	if len(vs) == 0 {
		vs = DefaultFiatCurrencies
	}
	vsStr := strings.Join(vs, ",")
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=%s", vsStr)
	resp, err := c.HTTPClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(url)
	if err != nil {
		return nil, fmt.Errorf("coingecko rates request failed: %w", err)
	}

	var rates map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal coingecko rates response: %w", err)
	}
	return rates, nil
}

func (c *CoinGeckoClient) GetBTCUSD(ctx context.Context) (float64, error) {
	v, ok := c.GetCryptoPriceInFiat(ctx, "bitcoin", "usd")
	if !ok {
		return 0, fmt.Errorf("coingecko bitcoin/usd unavailable")
	}
	return v, nil
}

// GetCryptoPriceInFiat returns the spot price of a crypto asset (by CoinGecko id) in the given fiat.
// Implements CryptoPriceFetcher for multi-asset valuation.
func (c *CoinGeckoClient) GetCryptoPriceInFiat(ctx context.Context, coinID, fiat string) (float64, bool) {
	fiat = strings.ToLower(strings.TrimSpace(fiat))
	if fiat == "" || !SupportedFiatCurrencies[fiat] {
		fiat = "usd"
	}
	coinID = strings.ToLower(strings.TrimSpace(coinID))
	if coinID == "" {
		return 0, false
	}
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=%s", coinID, fiat)
	resp, err := c.HTTPClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(url)
	if err != nil {
		return 0, false
	}
	var rates map[string]map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &rates); err != nil {
		return 0, false
	}
	coin, ok := rates[coinID]
	if !ok {
		return 0, false
	}
	val, ok := coin[fiat]
	if !ok {
		return 0, false
	}
	switch v := val.(type) {
	case float64:
		return v, v >= 0
	case int:
		return float64(v), v >= 0
	default:
		return 0, false
	}
}
