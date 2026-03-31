package export

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/pricing"
)

// WritePortfolioHoldingsCSV writes portfolio rows with valuation when resolve returns prices.
func WritePortfolioHoldingsCSV(ctx context.Context, w io.Writer, valuationCurrency string, usdPerFiat float64, created, updated time.Time, p *PortfolioSnapshot, resolve PriceResolver) error {
	if p == nil {
		return nil
	}
	cw := csv.NewWriter(w)
	createdStr := created.UTC().Format(time.RFC3339)
	updatedStr := updated.UTC().Format(time.RFC3339)
	_ = cw.Write([]string{"symbol", "type", "address", "amount", "value_" + valuationCurrency, "portfolio_created", "portfolio_updated"})
	for _, item := range p.Items {
		amountStr := strconv.FormatFloat(item.Amount, 'f', -1, 64)
		valueStr := ""
		assetType := strings.ToLower(strings.TrimSpace(item.Type))
		symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
		if resolve != nil {
			if price, ok := resolve(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
				valueStr = strconv.FormatFloat(item.Amount*price, 'f', 2, 64)
			}
		}
		_ = cw.Write([]string{
			item.Label,
			item.Type,
			item.Address,
			amountStr,
			valueStr,
			createdStr,
			updatedStr,
		})
	}
	cw.Flush()
	return cw.Error()
}
