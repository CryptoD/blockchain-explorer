package pricing

import (
	"strings"
)

// USDPerUnitOfFiatFromBTCRates derives how many USD one unit of fiat is worth using
// CoinGecko-style simple/price maps: rates["bitcoin"] is a map of vs_currencies to numbers.
// For example, if 1 BTC = 50_000 USD and 1 BTC = 45_000 EUR, then 1 EUR ≈ 50_000/45_000 USD.
func USDPerUnitOfFiatFromBTCRates(rates map[string]interface{}, fiat string) (float64, bool) {
	fiat = strings.ToLower(strings.TrimSpace(fiat))
	if fiat == "" || !IsSupportedFiat(fiat) {
		fiat = "usd"
	}
	if fiat == "usd" {
		return 1, true
	}
	btc, ok := rates["bitcoin"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	var btcUSD, btcFiat float64
	for k, val := range btc {
		k = strings.ToLower(strings.TrimSpace(k))
		var v float64
		switch x := val.(type) {
		case float64:
			v = x
		case int:
			v = float64(x)
		default:
			continue
		}
		if k == "usd" {
			btcUSD = v
		}
		if k == fiat {
			btcFiat = v
		}
	}
	if btcUSD <= 0 || btcFiat <= 0 {
		return 0, false
	}
	return btcUSD / btcFiat, true
}
