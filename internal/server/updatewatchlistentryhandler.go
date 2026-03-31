package server

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/export"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/gin-gonic/gin"
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

	snap := portfolioExportSnapshot(p)
	if err := export.WritePortfolioHoldingsCSV(ctx, c.Writer, valuationCurrency, usdPerFiat, p.Created, p.Updated, snap, exportPriceResolver()); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "csv_write_failed").Error("CSV export write failed")
	}
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
	snap := portfolioExportSnapshot(p)
	if err := export.WritePortfolioPDF(ctx, c.Writer, snap, valuationCurrency, usdPerFiat, exportPriceResolver()); err != nil {
		logging.WithComponent(logging.ComponentExport).WithError(err).WithField(logging.FieldEvent, "pdf_export_failed").Error("portfolio PDF export failed")
		errorResponse(c, http.StatusInternalServerError, "pdf_generation_failed", "Failed to generate PDF")
		return
	}
}

// exportBlocksCSVHandler streams blocks in a height range as CSV. Memory-efficient: one block at a time.
// Query params: start_height, end_height (required), limit (optional, default 500, max 2000).
// Range is capped at export.MaxBlockRange blocks.
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
	if rangeSize > export.MaxBlockRange {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range may not exceed %d blocks", export.MaxBlockRange))
		return
	}
	limit := export.DefaultBlockRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > export.MaxBlockRows {
				n = export.MaxBlockRows
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
	_ = export.WriteBlocksCSVHeader(w)
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		height := int(export.Float64(block["height"]))
		if height == 0 {
			height = h
		}
		hash := export.String(block["hash"])
		timeVal := export.Float64(block["time"])
		tm := time.Unix(int64(timeVal), 0).UTC()
		txCount := 0
		if txs, ok := block["tx"].([]interface{}); ok {
			txCount = len(txs)
		}
		size := export.Float64(block["size"])
		weight := export.Float64(block["weight"])
		difficulty := export.Float64(block["difficulty"])
		confs := best - height + 1
		if confs < 0 {
			confs = 0
		}
		_ = export.WriteBlockRow(w, height, hash, timeVal, tm, txCount, size, weight, difficulty, confs)
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
	if rangeSize > export.MaxTxBlockRange {
		errorResponse(c, http.StatusBadRequest, "range_too_large", fmt.Sprintf("block range for transaction export may not exceed %d blocks", export.MaxTxBlockRange))
		return
	}
	limit := export.DefaultTxRows
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > export.MaxTxRows {
				n = export.MaxTxRows
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
	_ = export.WriteTransactionsCSVHeader(w)
	written := 0
	for h := start; h <= end && written < limit; h++ {
		block, err := getBlockDetails(fmt.Sprintf("%d", h))
		if err != nil {
			continue
		}
		blockHash := export.String(block["hash"])
		blockTime := export.Float64(block["time"])
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
			size := export.Float64(tx["size"])
			vsize := export.Float64(tx["vsize"])
			weight := export.Float64(tx["weight"])
			fee := export.Float64(tx["fee"])
			locktime := export.Float64(tx["locktime"])
			version := export.Float64(tx["version"])
			_ = export.WriteTransactionRow(w, txid, h, blockHash, blockTime, blockTimeISO, size, vsize, weight, fee, locktime, version)
			written++
		}
	}
	w.Flush()
	if w.Error() != nil {
		logging.WithComponent(logging.ComponentExport).WithError(w.Error()).WithField(logging.FieldEvent, "csv_write_failed").Error("transactions CSV export write failed")
	}
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
			"export_version":      export.Version,
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
