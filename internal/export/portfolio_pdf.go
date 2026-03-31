package export

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/jung-kurt/gofpdf/v2"
)

// WritePortfolioPDF writes a portfolio summary report to w using gofpdf.
func WritePortfolioPDF(ctx context.Context, w io.Writer, p *PortfolioSnapshot, valuationCurrency string, usdPerFiat float64, resolve PriceResolver) error {
	if p == nil {
		return fmt.Errorf("export: nil portfolio")
	}
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, p.Name, "", 0, "L", false, 0, "")
	pdf.Ln(12)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(0, 6, "Portfolio Report — Generated "+time.Now().UTC().Format("2006-01-02 15:04 MST"), "", 0, "L", false, 0, "")
	pdf.Ln(10)

	price := func(assetType, symbol, fiat string) (float64, bool) {
		if resolve == nil {
			return 0, false
		}
		return resolve(ctx, assetType, symbol, fiat, usdPerFiat)
	}

	var totalQty float64
	for _, item := range p.Items {
		totalQty += item.Amount
	}
	totalFiat := 0.0
	hasAnyRate := false
	if valuationCurrency != "" && resolve != nil {
		for _, item := range p.Items {
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if pv, ok := price(assetType, symbol, valuationCurrency); ok && pv >= 0 {
				totalFiat += item.Amount * pv
				hasAnyRate = true
			}
		}
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Summary", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 10)
	summaryLine := fmt.Sprintf("Total (quantity): %s  |  Positions: %d  |  Created: %s  |  Updated: %s",
		FormatFloat(totalQty), len(p.Items),
		p.Created.UTC().Format("2006-01-02"),
		p.Updated.UTC().Format("2006-01-02"))
	if hasAnyRate && valuationCurrency != "" && totalFiat > 0 {
		summaryLine += fmt.Sprintf("  |  Total value (%s): %s", strings.ToUpper(valuationCurrency), FormatFloat(totalFiat))
	}
	pdf.CellFormat(0, 6, summaryLine, "", 0, "L", false, 0, "")
	pdf.Ln(12)

	typeAlloc := make(map[string]float64)
	for _, item := range p.Items {
		t := strings.ToLower(strings.TrimSpace(item.Type))
		if t == "" {
			t = "other"
		}
		typeAlloc[t] += item.Amount
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Allocations by asset type", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	colW := []float64{35, 45, 25, 50}
	headers := []string{"Type", "Amount", "%", "Bar"}
	for i, h := range headers {
		pdf.CellFormat(colW[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	totalForPct := totalQty
	if totalForPct == 0 {
		totalForPct = 1
	}
	for _, t := range []string{"crypto", "stock", "bond", "commodity", "other"} {
		amt, ok := typeAlloc[t]
		if !ok || amt == 0 {
			continue
		}
		pct := amt / totalForPct * 100
		pdf.CellFormat(colW[0], 6, t, "1", 0, "L", false, 0, "")
		pdf.CellFormat(colW[1], 6, FormatFloat(amt), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[2], 6, FormatFloat(pct)+"%", "1", 0, "R", false, 0, "")
		barX, barY := pdf.GetX(), pdf.GetY()
		pdf.CellFormat(colW[3], 6, "", "1", 0, "L", false, 0, "")
		barW := (colW[3] - 2) * (pct / 100)
		if barW > 0.5 {
			pdf.Rect(barX+1, barY+0.5, barW, 5, "F")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(8)

	posColW := []float64{45, 25, 70, 35, 40}
	posHeaders := []string{"Label", "Type", "Address", "Amount", "Value (" + strings.ToUpper(valuationCurrency) + ")"}
	if !hasAnyRate || valuationCurrency == "" {
		posColW = []float64{45, 25, 70, 35}
		posHeaders = []string{"Label", "Type", "Address", "Amount"}
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Positions", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 9)
	for i, h := range posHeaders {
		pdf.CellFormat(posColW[i], 7, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	for _, item := range p.Items {
		label := item.Label
		if len(label) > 28 {
			label = label[:25] + "..."
		}
		addr := item.Address
		if len(addr) > 38 {
			addr = addr[:35] + "..."
		}
		pdf.CellFormat(posColW[0], 6, label, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[1], 6, item.Type, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[2], 6, addr, "1", 0, "L", false, 0, "")
		pdf.CellFormat(posColW[3], 6, FormatFloat(item.Amount), "1", 0, "R", false, 0, "")
		if hasAnyRate && valuationCurrency != "" && len(posColW) > 4 {
			valStr := ""
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if pv, ok := price(assetType, symbol, valuationCurrency); ok && pv >= 0 {
				valStr = FormatFloat(item.Amount * pv)
			}
			pdf.CellFormat(posColW[4], 6, valStr, "1", 0, "R", false, 0, "")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "I", 8)
	footer := "Performance history is not available in this report."
	if hasAnyRate && valuationCurrency != "" {
		footer = "Values in " + strings.ToUpper(valuationCurrency) + " use current rates (crypto, commodity, bond); missing rate data is shown blank."
	} else {
		footer += " Value above is total quantity (amounts)."
	}
	pdf.CellFormat(0, 5, footer, "", 0, "L", false, 0, "")

	return pdf.Output(w)
}
