package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client defines an interface for retrieving pricing/FX data.
type Client interface {
	// GetMultiCurrencyRates returns rates for bitcoin in multiple fiat currencies.
	GetMultiCurrencyRates(ctx context.Context) (map[string]interface{}, error)
	// GetBTCUSD returns the current BTC/USD spot price.
	GetBTCUSD(ctx context.Context) (float64, error)
}

// CoinGeckoClient is a concrete implementation of Client using the CoinGecko API.
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
	resp, err := c.HTTPClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd,eur,gbp,jpy")
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
	resp, err := c.HTTPClient.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd")
	if err != nil {
		return 0, fmt.Errorf("coingecko BTC/USD request failed: %w", err)
	}

	var rates map[string]map[string]float64
	if err := json.Unmarshal(resp.Body(), &rates); err != nil {
		return 0, fmt.Errorf("failed to unmarshal coingecko BTC/USD response: %w", err)
	}
	btc, ok := rates["bitcoin"]
	if !ok {
		return 0, fmt.Errorf("missing bitcoin key in coingecko response")
	}
	usd, ok := btc["usd"]
	if !ok {
		return 0, fmt.Errorf("missing usd price in coingecko response")
	}
	return usd, nil
}

