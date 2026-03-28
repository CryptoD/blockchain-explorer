package server

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf/v2"
)

func updateWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	indexStr := c.Param("index")
	if id == "" || indexStr == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID and entry index required")
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_index", "Entry index must be a non-negative integer")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	var e WatchlistEntry
	if err := c.ShouldBindJSON(&e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	if err := validateWatchlistEntry(0, &e); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
		return
	}
	w, err := watchlistSvc.GetWatchlist(ctx, uname, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		}
		return
	}
	if index >= len(w.Entries) {
		errorResponse(c, http.StatusNotFound, "entry_not_found", "Entry index out of range")
		return
	}
	w.Entries[index] = e
	w.Updated = time.Now()
	if err := watchlistSvc.SaveWatchlist(ctx, w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, w)
}

// deleteWatchlistEntryHandler removes the entry at the given 0-based index.
func deleteWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	indexStr := c.Param("index")
	if id == "" || indexStr == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID and entry index required")
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_index", "Entry index must be a non-negative integer")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	w, err := watchlistSvc.GetWatchlist(ctx, uname, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		}
		return
	}
	if index >= len(w.Entries) {
		errorResponse(c, http.StatusNotFound, "entry_not_found", "Entry index out of range")
		return
	}
	w.Entries = append(w.Entries[:index], w.Entries[index+1:]...)
	w.Updated = time.Now()
	if err := watchlistSvc.SaveWatchlist(ctx, w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, w)
}

// exportPortfolioCSVHandler streams a single portfolio's holdings as CSV.
// Requires authentication. Sets Content-Type and Content-Disposition for browser download.
func exportPortfolioCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")
	portfolioID := c.Param("id")
	p, err := portfolioSvc.GetPortfolio(ctx, username.(string), portfolioID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to load portfolio")
		}
		return
	}

	// Safe filename: alphanumeric, dash, underscore only
	var b strings.Builder
	for _, r := range p.Name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ' {
			if r == ' ' {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	safeName := b.String()
	if safeName == "" {
		safeName = "portfolio"
	}
	filename := fmt.Sprintf("portfolio-%s-%s.csv", portfolioID, safeName)
	if len(filename) > 200 {
		filename = "portfolio-" + portfolioID + ".csv"
	}

	if len(p.Items) > 20 {
		logLargeExport(c, "portfolios/:id/export/csv", map[string]interface{}{"portfolio_id": portfolioID, "item_count": len(p.Items)})
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"symbol", "type", "address", "amount", "value_" + valuationCurrency, "portfolio_created", "portfolio_updated"})
	createdStr := p.Created.UTC().Format(time.RFC3339)
	updatedStr := p.Updated.UTC().Format(time.RFC3339)
	for _, item := range p.Items {
		amountStr := strconv.FormatFloat(item.Amount, 'f', -1, 64)
		valueStr := ""
		assetType := strings.ToLower(strings.TrimSpace(item.Type))
		symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
		if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
			valueStr = strconv.FormatFloat(item.Amount*price, 'f', 2, 64)
		}
		_ = w.Write([]string{
			item.Label,
			item.Type,
			item.Address,
			amountStr,
			valueStr,
			createdStr,
			updatedStr,
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "csv_write_failed").Error("CSV export write failed")
	}
}

// generatePortfolioPDF writes a short portfolio summary report (overall value, allocations by type, positions table) to w.
// Uses unified asset pricer for crypto, commodity, bond; usdPerFiat converts USD to user fiat when needed.
func generatePortfolioPDF(p *Portfolio, w io.Writer, valuationCurrency string, usdPerFiat float64) error {
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

	// Overall value: quantity and total fiat (from unified pricer)
	var totalQty float64
	for _, item := range p.Items {
		totalQty += item.Amount
	}
	totalFiat := 0.0
	hasAnyRate := false
	if valuationCurrency != "" && assetPricer != nil {
		for _, item := range p.Items {
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
				totalFiat += item.Amount * price
				hasAnyRate = true
			}
		}
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.CellFormat(0, 8, "Summary", "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Helvetica", "", 10)
	summaryLine := fmt.Sprintf("Total (quantity): %s  |  Positions: %d  |  Created: %s  |  Updated: %s",
		formatFloat(totalQty), len(p.Items),
		p.Created.UTC().Format("2006-01-02"),
		p.Updated.UTC().Format("2006-01-02"))
	if hasAnyRate && valuationCurrency != "" && totalFiat > 0 {
		summaryLine += fmt.Sprintf("  |  Total value (%s): %s", strings.ToUpper(valuationCurrency), formatFloat(totalFiat))
	}
	pdf.CellFormat(0, 6, summaryLine, "", 0, "L", false, 0, "")
	pdf.Ln(12)

	// Allocations by asset type (by quantity)
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
		pdf.CellFormat(colW[1], 6, formatFloat(amt), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colW[2], 6, formatFloat(pct)+"%", "1", 0, "R", false, 0, "")
		barX, barY := pdf.GetX(), pdf.GetY()
		pdf.CellFormat(colW[3], 6, "", "1", 0, "L", false, 0, "")
		barW := (colW[3] - 2) * (pct / 100)
		if barW > 0.5 {
			pdf.Rect(barX+1, barY+0.5, barW, 5, "F")
		}
		pdf.Ln(-1)
	}
	pdf.Ln(8)

	// Positions table: add Value (fiat) column when rate available (unified pricer)
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
		pdf.CellFormat(posColW[3], 6, formatFloat(item.Amount), "1", 0, "R", false, 0, "")
		if hasAnyRate && valuationCurrency != "" && len(posColW) > 4 {
			valStr := ""
			assetType := strings.ToLower(strings.TrimSpace(item.Type))
			symbol := pricing.NormalizeAssetSymbol(item.Type, item.Symbol)
			if price, ok := getAssetPriceInFiat(ctx, assetType, symbol, valuationCurrency, usdPerFiat); ok && price >= 0 {
				valStr = formatFloat(item.Amount * price)
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

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	if f >= 1e6 || (f < 0.0001 && f > 0) {
		return strconv.FormatFloat(f, 'e', 2, 64)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// exportPortfolioPDFHandler generates and streams a portfolio summary report as PDF. Requires authentication.
func exportPortfolioPDFHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")
	portfolioID := c.Param("id")
	p, err := portfolioSvc.GetPortfolio(ctx, username.(string), portfolioID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to load portfolio")
		}
		return
	}
	if len(p.Items) > 20 {
		logLargeExport(c, "portfolios/:id/export/pdf", map[string]interface{}{"portfolio_id": portfolioID, "item_count": len(p.Items)})
	}
	var b strings.Builder
	for _, r := range p.Name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ' {
			if r == ' ' {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	safeName := b.String()
	if safeName == "" {
		safeName = "portfolio"
	}
	filename := fmt.Sprintf("portfolio-%s-%s.pdf", portfolioID, safeName)
	if len(filename) > 200 {
		filename = "portfolio-" + portfolioID + ".pdf"
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := generatePortfolioPDF(p, c.Writer, valuationCurrency, usdPerFiat); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "pdf_export_failed").Error("portfolio PDF export failed")
		errorResponse(c, http.StatusInternalServerError, "pdf_generation_failed", "Failed to generate PDF")
		return
	}
}

const exportVersion = "1.0"

// CSV export limits to prevent abuse and control memory/RPC load.
const (
	maxBlockRangeExport   = 500  // max blocks in one blocks CSV export (end_height - start_height + 1)
	maxBlockRowsExport    = 2000 // max rows for blocks CSV
	maxTxBlockRangeExport = 100  // max block range when exporting transactions (each block may have many txs)
	maxTxRowsExport       = 5000 // max transaction rows per CSV export
	defaultBlockRows      = 500
	defaultTxRows         = 1000
)

// exportBlocksCSVHandler streams blocks in a height range as CSV. Memory-efficient: one block at a time.
// Query params: start_height, end_height (required), limit (optional, default 500, max 2000).
// Range is capped at maxBlockRangeExport blocks.
func exportBlocksCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	startStr := c.Query("start_height")
	endStr := c.Query("end_height")
	if startStr == "" || endStr == "" {
		errorResponse(c, http.StatusBadRequest, "missing_range", "start_height and end_height are required")
		return
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start < 0 || end < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height and end_height must be non-negative integers")
		return
	}
	if start > end {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height must be <= end_height")
		return
	}
	rangeSize := end - start + 1
	if rangeSize > maxBlockRangeExport {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range may not exceed %d blocks", maxBlockRangeExport))
		return
	}
	limit := defaultBlockRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > maxBlockRowsExport {
				n = maxBlockRowsExport
			}
			limit = n
		}
	}

	status, err := getNetworkStatus()
	if err != nil {
		errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "could not get chain height")
		return
	}
	bestF, _ := status["block_height"].(float64)
	best := int(bestF)
	if end > best {
		errorResponse(c, http.StatusBadRequest, "invalid_range", fmt.Sprintf("end_height cannot exceed current chain height %d", best))
		return
	}
	if rangeSize >= 100 || limit >= 1000 {
		logLargeExport(c, "blocks/export/csv", map[string]interface{}{"start_height": start, "end_height": end, "range_size": rangeSize, "limit": limit})
	}
	filename := fmt.Sprintf("blocks-%d-%d.csv", start, end)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"height", "hash", "time", "time_iso", "tx_count", "size", "weight", "difficulty", "confirmations"})
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		height := int(float64OrZero(block["height"]))
		if height == 0 {
			height = h
		}
		hash := stringOrEmpty(block["hash"])
		timeVal := float64OrZero(block["time"])
		tm := time.Unix(int64(timeVal), 0).UTC()
		txCount := 0
		if txs, ok := block["tx"].([]interface{}); ok {
			txCount = len(txs)
		}
		size := float64OrZero(block["size"])
		weight := float64OrZero(block["weight"])
		difficulty := float64OrZero(block["difficulty"])
		confs := best - height + 1
		if confs < 0 {
			confs = 0
		}
		_ = w.Write([]string{
			fmt.Sprintf("%d", height),
			hash,
			fmt.Sprintf("%.0f", timeVal),
			tm.Format(time.RFC3339),
			fmt.Sprintf("%d", txCount),
			fmt.Sprintf("%.0f", size),
			fmt.Sprintf("%.0f", weight),
			fmt.Sprintf("%.0f", difficulty),
			fmt.Sprintf("%d", confs),
		})
		written++
	}
	w.Flush()
	if w.Error() != nil {
		logging.WithComponent(logging.ComponentExport).WithError(w.Error()).WithField(logging.FieldEvent, "csv_write_failed").Error("blocks CSV export write failed")
	}
}

// exportTransactionsCSVHandler streams transactions from blocks in a height range as CSV.
// One block at a time, then one tx at a time per block. Query params: start_height, end_height (required), limit (optional, default 1000, max 5000).
// Uses stricter (heavy) export rate limit due to RPC load.
func exportTransactionsCSVHandler(c *gin.Context) {
	if !checkExportRateLimit(c, true) {
		return
	}
	startStr := c.Query("start_height")
	endStr := c.Query("end_height")
	if startStr == "" || endStr == "" {
		errorResponse(c, http.StatusBadRequest, "missing_range", "start_height and end_height are required")
		return
	}
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start < 0 || end < 0 {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height and end_height must be non-negative integers")
		return
	}
	if start > end {
		errorResponse(c, http.StatusBadRequest, "invalid_range", "start_height must be <= end_height")
		return
	}
	rangeSize := end - start + 1
	if rangeSize > maxTxBlockRangeExport {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range for transaction export may not exceed %d blocks", maxTxBlockRangeExport))
		return
	}
	limit := defaultTxRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > maxTxRowsExport {
				n = maxTxRowsExport
			}
			limit = n
		}
	}

	status, err := getNetworkStatus()
	if err != nil {
		errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "could not get chain height")
		return
	}
	bestF, _ := status["block_height"].(float64)
	best := int(bestF)
	if end > best {
		errorResponse(c, http.StatusBadRequest, "invalid_range", fmt.Sprintf("end_height cannot exceed current chain height %d", best))
		return
	}
	if rangeSize >= 50 || limit >= 2000 {
		logLargeExport(c, "transactions/export/csv", map[string]interface{}{"start_height": start, "end_height": end, "range_size": rangeSize, "limit": limit})
	}
	filename := fmt.Sprintf("transactions-%d-%d.csv", start, end)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"txid", "block_height", "block_hash", "block_time", "block_time_iso", "size", "vsize", "weight", "fee", "locktime", "version"})
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		blockHash := stringOrEmpty(block["hash"])
		blockTime := float64OrZero(block["time"])
		blockTimeISO := time.Unix(int64(blockTime), 0).UTC().Format(time.RFC3339)
		txList, ok := block["tx"].([]interface{})
		if !ok {
			continue
		}
		for _, txi := range txList {
			if written >= limit {
				break
			}
			txid, _ := txi.(string)
			if txid == "" {
				continue
			}
			tx, err := getTransactionDetails(txid)
			if err != nil {
				continue
			}
			size := float64OrZero(tx["size"])
			vsize := float64OrZero(tx["vsize"])
			weight := float64OrZero(tx["weight"])
			fee := float64OrZero(tx["fee"])
			locktime := float64OrZero(tx["locktime"])
			version := float64OrZero(tx["version"])
			_ = w.Write([]string{
				txid,
				fmt.Sprintf("%d", h),
				blockHash,
				fmt.Sprintf("%.0f", blockTime),
				blockTimeISO,
				fmt.Sprintf("%.0f", size),
				fmt.Sprintf("%.0f", vsize),
				fmt.Sprintf("%.0f", weight),
				fmt.Sprintf("%.6f", fee),
				fmt.Sprintf("%.0f", locktime),
				fmt.Sprintf("%.0f", version),
			})
			written++
		}
	}
	w.Flush()
	if w.Error() != nil {
		logging.WithComponent(logging.ComponentExport).WithError(w.Error()).WithField(logging.FieldEvent, "csv_write_failed").Error("transactions CSV export write failed")
	}
}

func float64OrZero(v interface{}) float64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

func stringOrEmpty(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// exportPortfoliosHandler returns portfolios as machine-friendly JSON for archival or analysis.
// Requires authentication. Respects pagination (page, page_size) and sort (sort_by, sort_dir).
func exportPortfoliosHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	username, _ := c.Get("username")

	portfolios, err := portfolioSvc.ListPortfolios(ctx, username.(string))
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to fetch portfolios")
		return
	}

	sortParams := apiutil.ParseSort(c, "created", "desc", map[string]bool{
		"created": true,
		"updated": true,
	})

	switch sortParams.Field {
	case "updated":
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Updated.Before(portfolios[j+1].Updated)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	default:
		for i := 0; i < len(portfolios)-1; i++ {
			for j := 0; j < len(portfolios)-i-1; j++ {
				swap := portfolios[j].Created.Before(portfolios[j+1].Created)
				if sortParams.Direction == "asc" {
					swap = !swap
				}
				if swap {
					portfolios[j], portfolios[j+1] = portfolios[j+1], portfolios[j]
				}
			}
		}
	}

	pagination := apiutil.ParsePagination(c, 20, 100)
	total := len(portfolios)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginated := portfolios[start:end]
	if total >= 50 || pagination.PageSize >= 50 {
		logLargeExport(c, "portfolios/export", map[string]interface{}{"total": total, "page_size": pagination.PageSize})
	}
	valuationCurrency := "usd"
	if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
		valuationCurrency = strings.ToLower(u.PreferredCurrency)
	}
	usdPerFiat := 1.0
	if valuationCurrency != "usd" {
		if u, ok := getUSDPerFiat(ctx, valuationCurrency); ok && u > 0 {
			usdPerFiat = u
		}
	}
	dataWithValuation := make([]PortfolioWithValuation, 0, len(paginated))
	for i := range paginated {
		p := &paginated[i]
		totalFiat, itemsWithVal := computePortfolioValuation(p, valuationCurrency, usdPerFiat)
		dataWithValuation = append(dataWithValuation, PortfolioWithValuation{
			ID:                p.ID,
			Username:          p.Username,
			Name:              p.Name,
			Description:       p.Description,
			Created:           p.Created,
			Updated:           p.Updated,
			ValuationCurrency: valuationCurrency,
			TotalValueFiat:    totalFiat,
			Items:             itemsWithVal,
		})
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp":    time.Now().UTC().Format(time.RFC3339),
			"export_version":      exportVersion,
			"endpoint":            "portfolios",
			"valuation_currency":  valuationCurrency,
			"rate_data_available": assetPricer != nil,
		},
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"data": dataWithValuation,
	})
}
