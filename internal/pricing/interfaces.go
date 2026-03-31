package pricing

import "context"

// Client defines an interface for retrieving pricing/FX data (BTC vs fiat) from an external provider.
type Client interface {
	// GetMultiCurrencyRates returns rates for bitcoin in the default fiat currencies.
	GetMultiCurrencyRates(ctx context.Context) (map[string]interface{}, error)
	// GetMultiCurrencyRatesIn returns rates for bitcoin in the requested fiat currencies (normalized, lowercase).
	// If currencies is empty, uses DefaultFiatCurrencies. Invalid codes are ignored.
	GetMultiCurrencyRatesIn(ctx context.Context, currencies []string) (map[string]interface{}, error)
	// GetBTCUSD returns the current BTC/USD spot price.
	GetBTCUSD(ctx context.Context) (float64, error)
}

// AssetPricer returns the spot price of an asset in the given fiat currency.
// symbol is asset-specific (e.g. "bitcoin", "XAU", "US10Y"). Returns (0, false) when unavailable.
// usdPerFiat: 1 unit of fiat = usdPerFiat USD (e.g. 1 EUR = 1.08 USD). Use 1.0 when fiat is USD.
// Used to convert commodity/bond USD prices into user fiat when fiat != USD.
type AssetPricer interface {
	GetAssetPriceInFiat(ctx context.Context, assetType, symbol, fiat string, usdPerFiat float64) (float64, bool)
}

// CryptoPriceFetcher returns the price of a crypto asset (by CoinGecko id) in fiat.
type CryptoPriceFetcher interface {
	GetCryptoPriceInFiat(ctx context.Context, coinID, fiat string) (float64, bool)
}

// CommodityPriceFetcher returns the price of a commodity (e.g. XAU, XAG) in fiat.
type CommodityPriceFetcher interface {
	GetCommodityPriceInFiat(ctx context.Context, symbol, fiat string) (float64, bool)
}

// BondPriceFetcher returns the price of a bond (e.g. US10Y) in fiat.
type BondPriceFetcher interface {
	GetBondPriceInFiat(ctx context.Context, symbol, fiat string) (float64, bool)
}
