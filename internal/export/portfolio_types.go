package export

import (
	"context"
	"time"
)

// PortfolioSnapshot is a minimal portfolio view for PDF/CSV generation (no JSON tags required).
type PortfolioSnapshot struct {
	Name    string
	Created time.Time
	Updated time.Time
	Items   []PortfolioItemSnapshot
}

// PortfolioItemSnapshot is one row for export.
type PortfolioItemSnapshot struct {
	Label   string
	Type    string
	Address string
	Symbol  string
	Amount  float64
}

// PriceResolver returns unit price in the given fiat for valuation (same contract as server getAssetPriceInFiat).
type PriceResolver func(ctx context.Context, assetType, symbol, fiat string, usdPerFiat float64) (float64, bool)
