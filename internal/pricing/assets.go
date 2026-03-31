package pricing

import (
	"context"
	"strings"
)

// Asset class constants for portfolio items.
const (
	AssetClassCrypto    = "crypto"
	AssetClassCommodity = "commodity"
	AssetClassBond      = "bond"
	AssetClassStock     = "stock"
)

// CryptoIDBySymbol maps common symbols to CoinGecko API ids.
var CryptoIDBySymbol = map[string]string{
	"btc": "bitcoin", "bitcoin": "bitcoin",
	"eth": "ethereum", "ethereum": "ethereum",
	"usdt": "tether", "tether": "tether",
	"bnb": "binancecoin", "binancecoin": "binancecoin",
	"sol": "solana", "solana": "solana",
	"xrp": "ripple", "ripple": "ripple",
	"usdc": "usd-coin", "usd-coin": "usd-coin",
	"ada": "cardano", "cardano": "cardano",
	"avax": "avalanche-2", "avalanche-2": "avalanche-2",
	"doge": "dogecoin", "dogecoin": "dogecoin",
	"link": "chainlink", "chainlink": "chainlink",
	"uni": "uniswap", "uniswap": "uniswap",
	"aave": "aave", "matic": "matic-network", "matic-network": "matic-network",
	"dot": "polkadot", "polkadot": "polkadot",
	"ltc": "litecoin", "litecoin": "litecoin",
	"atom": "cosmos", "cosmos": "cosmos",
}

// NormalizeAssetSymbol returns a lowercase symbol for lookup; empty defaults for crypto to "bitcoin".
func NormalizeAssetSymbol(assetType, symbol string) string {
	s := strings.ToLower(strings.TrimSpace(symbol))
	t := strings.ToLower(strings.TrimSpace(assetType))
	if t == AssetClassCrypto && s == "" {
		return "bitcoin"
	}
	return s
}

// CompositePricer delegates to crypto, commodity, and bond sources by asset type.
type CompositePricer struct {
	Crypto    CryptoPriceFetcher
	Commodity CommodityPriceFetcher
	Bond      BondPriceFetcher
}

// GetAssetPriceInFiat implements AssetPricer.
// When asset is commodity or bond, price is in USD; if fiat != USD and usdPerFiat > 0, result is converted (priceUSD / usdPerFiat).
func (c *CompositePricer) GetAssetPriceInFiat(ctx context.Context, assetType, symbol, fiat string, usdPerFiat float64) (float64, bool) {
	fiat = strings.ToLower(strings.TrimSpace(fiat))
	if fiat == "" || !SupportedFiatCurrencies[fiat] {
		fiat = "usd"
	}
	if usdPerFiat <= 0 {
		usdPerFiat = 1
	}
	assetType = strings.ToLower(strings.TrimSpace(assetType))
	symbol = NormalizeAssetSymbol(assetType, symbol)
	if symbol == "" {
		return 0, false
	}

	switch assetType {
	case AssetClassCrypto:
		if c.Crypto == nil {
			return 0, false
		}
		coinID, ok := CryptoIDBySymbol[symbol]
		if !ok {
			coinID = symbol
		}
		return c.Crypto.GetCryptoPriceInFiat(ctx, coinID, fiat)
	case AssetClassCommodity:
		if c.Commodity == nil {
			return 0, false
		}
		priceUSD, ok := c.Commodity.GetCommodityPriceInFiat(ctx, strings.ToUpper(symbol), "USD")
		if !ok {
			return 0, false
		}
		if fiat == "usd" {
			return priceUSD, true
		}
		return priceUSD / usdPerFiat, true
	case AssetClassBond:
		if c.Bond == nil {
			return 0, false
		}
		priceUSD, ok := c.Bond.GetBondPriceInFiat(ctx, strings.ToUpper(symbol), "USD")
		if !ok {
			return 0, false
		}
		if fiat == "usd" {
			return priceUSD, true
		}
		return priceUSD / usdPerFiat, true
	default:
		return 0, false
	}
}
