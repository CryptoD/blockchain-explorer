package pricing

import (
	"context"
	"strings"
)

// Supported bond symbols (e.g. US10Y = US 10-year Treasury).
var BondSymbols = map[string]bool{
	"US10Y": true, "US02Y": true, "US05Y": true, "US30Y": true,
}

// StaticBondSource provides bond prices from a static map (e.g. par = 100).
// Real-world bond pricing would use yield curves; this allows portfolio valuation with a placeholder.
type StaticBondSource struct {
	// PricePer100 is price per 100 face (e.g. 100 = par). Keys like "US10Y".
	PricePer100 map[string]float64
}

// DefaultBondPrices returns par (100) for common government bonds.
func DefaultBondPrices() map[string]float64 {
	return map[string]float64{
		"US10Y": 100,
		"US02Y": 100,
		"US05Y": 100,
		"US30Y": 100,
	}
}

// GetBondPriceInFiat returns price in fiat. Only USD supported; value is per 100 face.
func (b *StaticBondSource) GetBondPriceInFiat(ctx context.Context, symbol, fiat string) (float64, bool) {
	fiat = strings.ToUpper(strings.TrimSpace(fiat))
	if fiat != "USD" {
		return 0, false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !BondSymbols[symbol] {
		return 0, false
	}
	prices := b.PricePer100
	if prices == nil {
		prices = DefaultBondPrices()
	}
	price, ok := prices[symbol]
	return price, ok && price > 0
}
