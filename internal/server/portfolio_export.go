package server

import (
	"context"

	"github.com/CryptoD/blockchain-explorer/internal/export"
)

func portfolioExportSnapshot(p *Portfolio) *export.PortfolioSnapshot {
	if p == nil {
		return nil
	}
	items := make([]export.PortfolioItemSnapshot, len(p.Items))
	for i := range p.Items {
		it := p.Items[i]
		items[i] = export.PortfolioItemSnapshot{
			Label:   it.Label,
			Type:    it.Type,
			Address: it.Address,
			Symbol:  it.Symbol,
			Amount:  it.Amount,
		}
	}
	return &export.PortfolioSnapshot{
		Name:    p.Name,
		Created: p.Created,
		Updated: p.Updated,
		Items:   items,
	}
}

func exportPriceResolver() export.PriceResolver {
	return func(ctx context.Context, assetType, symbol, fiat string, usdPerFiat float64) (float64, bool) {
		return getAssetPriceInFiat(ctx, assetType, symbol, fiat, usdPerFiat)
	}
}
