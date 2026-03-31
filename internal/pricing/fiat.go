package pricing

import "strings"

// DefaultFiatCurrencies is the default set of fiat currencies for multi-currency rates.
var DefaultFiatCurrencies = []string{"usd", "eur", "gbp", "jpy", "cad", "aud", "chf"}

// SupportedFiatCurrencies is the set of fiat currencies we allow (subset of CoinGecko supported).
var SupportedFiatCurrencies = map[string]bool{
	"usd": true, "eur": true, "gbp": true, "jpy": true, "cad": true, "aud": true, "chf": true,
	"krw": true, "cny": true, "inr": true, "brl": true, "mxn": true, "try": true,
}

// IsSupportedFiat reports whether code is an allowed lowercase fiat currency.
func IsSupportedFiat(code string) bool {
	return SupportedFiatCurrencies[strings.ToLower(strings.TrimSpace(code))]
}

// NormalizeAndFilterFiatCurrencies returns lowercase, supported currency codes only (deduplicated).
func NormalizeAndFilterFiatCurrencies(currencies []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range currencies {
		code := strings.ToLower(strings.TrimSpace(s))
		if code == "" || seen[code] || !SupportedFiatCurrencies[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}
