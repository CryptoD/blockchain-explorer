package pricing

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// Supported commodity symbols (e.g. XAU = gold, XAG = silver).
var CommoditySymbols = map[string]bool{
	"XAU": true, "XAG": true, "XPT": true, "XPD": true,
}

// StaticCommoditySource provides commodity prices from env override or static defaults (per unit: oz for metals).
// Env: COMMODITY_XAU_USD, COMMODITY_XAG_USD, etc. override defaults. Useful when no external API is configured.
type StaticCommoditySource struct {
	// DefaultsUSD is optional; if nil, defaults below are used.
	DefaultsUSD map[string]float64
}

// DefaultCommodityPrices returns sensible fallback prices (USD per unit) when no API is available.
func DefaultCommodityPrices() map[string]float64 {
	return map[string]float64{
		"XAU": 2000,
		"XAG": 24,
		"XPT": 900,
		"XPD": 1000,
	}
}

// GetCommodityPriceInFiat returns price in requested fiat. Only USD is supported for static source; other fiat returns (0, false).
func (s *StaticCommoditySource) GetCommodityPriceInFiat(ctx context.Context, symbol, fiat string) (float64, bool) {
	fiat = strings.ToUpper(strings.TrimSpace(fiat))
	if fiat != "USD" {
		return 0, false
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if !CommoditySymbols[symbol] {
		return 0, false
	}
	if v := os.Getenv("COMMODITY_" + symbol + "_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f, true
		}
	}
	def := DefaultCommodityPrices()
	if s.DefaultsUSD != nil {
		def = s.DefaultsUSD
	}
	price, ok := def[symbol]
	return price, ok && price > 0
}
