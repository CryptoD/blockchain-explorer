package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/export"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func searchBlockchain(query string) (string, map[string]interface{}, error) {
	query = strings.TrimSpace(query)

	// Check if it's a valid Bitcoin address
	if isValidAddress(query) {
		addressDetails, err := getAddressDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "address", addressDetails, nil
	}

	// Check if it's a valid transaction ID
	if isValidTransactionID(query) {
		transactionDetails, err := getTransactionDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "transaction", transactionDetails, nil
	}

	// Check if it's a valid block height
	if isValidBlockHeight(query) {
		blockDetails, err := getBlockDetails(query)
		if err != nil {
			return "", nil, err
		}
		return "block", blockDetails, nil
	}

	// Check if it might be a block hash (64-char hex, but not a tx ID)
	if len(query) == 64 {
		matched, _ := regexp.MatchString("^[0-9a-fA-F]{64}$", query)
		if matched {
			blockDetails, err := getBlockDetails(query)
			if err == nil {
				return "block", blockDetails, nil
			}
		}
	}

	return "", nil, ErrNotFound
}

// Fix 1: Refactor searchHandler to use Gin's context
func searchHandler(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	qf := logging.QueryLogFields(query)
	if query != "" {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_request").Debug("search request received")
	}
	if query == "" {
		logging.WithComponent(logging.ComponentSearch).WithField(logging.FieldEvent, "search_empty_query").Warn("search request with empty query")
		errorResponse(c, http.StatusBadRequest, "missing_query", "Missing query parameter")
		return
	}
	if len(query) > 100 {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_query_too_long").Warn("search request query too long")
		errorResponse(c, http.StatusBadRequest, "query_too_long", "Query too long")
		return
	}
	resultType, result, err := explorerSvc.SearchBlockchain(c.Request.Context(), query)
	if err != nil {
		logging.WithComponent(logging.ComponentSearch).WithError(err).WithFields(qf).WithField(logging.FieldEvent, "search_failed").Error("search failed")
		errorResponseFrom(c, err)
		return
	}
	// Marshal the result to JSON for ETag calculation
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		logging.WithComponent(logging.ComponentSearch).WithError(err).WithFields(qf).WithField(logging.FieldEvent, "marshal_failed").Error("failed to marshal search response")
		errorResponse(c, http.StatusInternalServerError, "marshal_failed", "Failed to marshal response")
		return
	}
	etag := fmt.Sprintf("\"%x\"", sha256.Sum256(jsonBytes))
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=60")
	if match := c.GetHeader("If-None-Match"); match == etag {
		logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "search_cache_hit").Debug("search cache hit")
		metrics.RecordSearchETag(true)
		c.Status(304)
		return
	}
	metrics.RecordSearchETag(false)
	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithFields(log.Fields{
		logging.FieldResult: resultType,
		logging.FieldEvent:  "search_success",
	}).Info("search completed")
	c.JSON(200, gin.H{"type": resultType, "result": result})
}

// exportSearchHandler returns blockchain search results as machine-friendly JSON for archival or analysis.
// Same parameters as GET /api/search (q required). Public endpoint; no auth required.
func exportSearchHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" {
		errorResponse(c, http.StatusBadRequest, "missing_query", "Missing query parameter")
		return
	}
	if len(query) > 100 {
		errorResponse(c, http.StatusBadRequest, "query_too_long", "Query too long")
		return
	}
	resultType, result, err := explorerSvc.SearchBlockchain(c.Request.Context(), query)
	if err != nil {
		errorResponseFrom(c, err)
		return
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp": time.Now().UTC().Format(time.RFC3339),
			"export_version":   export.Version,
			"endpoint":         "search",
			"query":            query,
		},
		"data": gin.H{
			"type":   resultType,
			"result": result,
		},
	})
}

func autocompleteHandler(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, gin.H{"suggestions": []map[string]string{}})
		return
	}

	suggestions := []map[string]string{}

	// Check if query looks like an address
	if isValidAddress(query) {
		suggestions = append(suggestions, map[string]string{"type": "address", "value": query, "label": query})
	}

	// Check if query looks like a transaction ID
	if isValidTransactionID(query) {
		suggestions = append(suggestions, map[string]string{"type": "tx", "value": query, "label": query})
	}

	// Check if query looks like a block height
	if isValidBlockHeight(query) {
		suggestions = append(suggestions, map[string]string{"type": "block", "value": query, "label": "Block " + query})
	}

	c.JSON(http.StatusOK, gin.H{"suggestions": suggestions})
}

// SymbolInfo represents a cryptocurrency or asset symbol
type SymbolInfo struct {
	Symbol      string  `json:"symbol"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`     // "crypto", "stock", "commodity", etc.
	Category    string  `json:"category"` // e.g., "layer1", "defi", "nft", "payment"
	MarketCap   float64 `json:"market_cap"`
	Price       float64 `json:"price"`
	Volume24h   float64 `json:"volume_24h"`
	Change24h   float64 `json:"change_24h"` // percentage change
	Rank        int     `json:"rank"`
	IsActive    bool    `json:"is_active"`
	ListedSince int64   `json:"listed_since"` // timestamp
}

// SearchFilters represents the filter parameters for symbol search
type SearchFilters struct {
	Types        []string `json:"types"`      // Filter by symbol types
	Categories   []string `json:"categories"` // Filter by categories
	MinPrice     float64  `json:"min_price"`  // Minimum price filter
	MaxPrice     float64  `json:"max_price"`  // Maximum price filter
	MinMarketCap float64  `json:"min_market_cap"`
	MaxMarketCap float64  `json:"max_market_cap"`
	IsActive     *bool    `json:"is_active"` // Filter by active status
}

// SortOptions represents sorting configuration
type SortOptions struct {
	Field     string `json:"field"`     // Field to sort by
	Direction string `json:"direction"` // "asc" or "desc"
}

// In-memory symbol database (in production, this would be a real database)
var (
	symbolDatabase = []SymbolInfo{
		{Symbol: "BTC", Name: "Bitcoin", Type: "crypto", Category: "layer1", MarketCap: 850000000000, Price: 45000.00, Volume24h: 25000000000, Change24h: 2.5, Rank: 1, IsActive: true, ListedSince: 1279408155},
		{Symbol: "ETH", Name: "Ethereum", Type: "crypto", Category: "layer1", MarketCap: 280000000000, Price: 2300.00, Volume24h: 15000000000, Change24h: -1.2, Rank: 2, IsActive: true, ListedSince: 1438905600},
		{Symbol: "USDT", Name: "Tether", Type: "crypto", Category: "stablecoin", MarketCap: 95000000000, Price: 1.00, Volume24h: 45000000000, Change24h: 0.01, Rank: 3, IsActive: true, ListedSince: 1420070400},
		{Symbol: "BNB", Name: "BNB", Type: "crypto", Category: "exchange", MarketCap: 45000000000, Price: 320.00, Volume24h: 1200000000, Change24h: 0.8, Rank: 4, IsActive: true, ListedSince: 1502928000},
		{Symbol: "SOL", Name: "Solana", Type: "crypto", Category: "layer1", MarketCap: 42000000000, Price: 98.00, Volume24h: 2500000000, Change24h: 5.2, Rank: 5, IsActive: true, ListedSince: 1584403200},
		{Symbol: "XRP", Name: "XRP", Type: "crypto", Category: "payment", MarketCap: 28000000000, Price: 0.52, Volume24h: 1800000000, Change24h: -0.5, Rank: 6, IsActive: true, ListedSince: 1386547200},
		{Symbol: "USDC", Name: "USD Coin", Type: "crypto", Category: "stablecoin", MarketCap: 26000000000, Price: 1.00, Volume24h: 8000000000, Change24h: 0.0, Rank: 7, IsActive: true, ListedSince: 1538352000},
		{Symbol: "ADA", Name: "Cardano", Type: "crypto", Category: "layer1", MarketCap: 15000000000, Price: 0.42, Volume24h: 450000000, Change24h: 1.8, Rank: 8, IsActive: true, ListedSince: 1506816000},
		{Symbol: "AVAX", Name: "Avalanche", Type: "crypto", Category: "layer1", MarketCap: 12000000000, Price: 32.00, Volume24h: 600000000, Change24h: 3.1, Rank: 9, IsActive: true, ListedSince: 1609459200},
		{Symbol: "DOGE", Name: "Dogecoin", Type: "crypto", Category: "meme", MarketCap: 11000000000, Price: 0.08, Volume24h: 900000000, Change24h: -2.1, Rank: 10, IsActive: true, ListedSince: 1388966400},
		{Symbol: "LINK", Name: "Chainlink", Type: "crypto", Category: "defi", MarketCap: 8500000000, Price: 14.50, Volume24h: 400000000, Change24h: 2.8, Rank: 11, IsActive: true, ListedSince: 1509494400},
		{Symbol: "UNI", Name: "Uniswap", Type: "crypto", Category: "defi", MarketCap: 5200000000, Price: 6.80, Volume24h: 250000000, Change24h: 4.2, Rank: 12, IsActive: true, ListedSince: 1600041600},
		{Symbol: "AAVE", Name: "Aave", Type: "crypto", Category: "defi", MarketCap: 1800000000, Price: 120.00, Volume24h: 120000000, Change24h: 1.5, Rank: 13, IsActive: true, ListedSince: 1609459200},
		{Symbol: "SUSHI", Name: "SushiSwap", Type: "crypto", Category: "defi", MarketCap: 450000000, Price: 1.80, Volume24h: 45000000, Change24h: -1.8, Rank: 14, IsActive: true, ListedSince: 1598918400},
		{Symbol: "COMP", Name: "Compound", Type: "crypto", Category: "defi", MarketCap: 380000000, Price: 52.00, Volume24h: 28000000, Change24h: 0.9, Rank: 15, IsActive: true, ListedSince: 1592179200},
		{Symbol: "MKR", Name: "Maker", Type: "crypto", Category: "defi", MarketCap: 1600000000, Price: 1750.00, Volume24h: 85000000, Change24h: -0.7, Rank: 16, IsActive: true, ListedSince: 1514764800},
		{Symbol: "YFI", Name: "yearn.finance", Type: "crypto", Category: "defi", MarketCap: 220000000, Price: 6600.00, Volume24h: 18000000, Change24h: 3.5, Rank: 17, IsActive: true, ListedSince: 1598918400},
		{Symbol: "SNX", Name: "Synthetix", Type: "crypto", Category: "defi", MarketCap: 650000000, Price: 2.10, Volume24h: 35000000, Change24h: -2.3, Rank: 18, IsActive: true, ListedSince: 1567296000},
		{Symbol: "CRV", Name: "Curve DAO Token", Type: "crypto", Category: "defi", MarketCap: 480000000, Price: 0.55, Volume24h: 42000000, Change24h: 1.2, Rank: 19, IsActive: true, ListedSince: 1593561600},
		{Symbol: "BAL", Name: "Balancer", Type: "crypto", Category: "defi", MarketCap: 180000000, Price: 3.20, Volume24h: 12000000, Change24h: -0.5, Rank: 20, IsActive: true, ListedSince: 1590969600},
	}
	symbolDBMutex sync.RWMutex
)

// parseSearchFilters parses filter parameters from the request
func parseSearchFilters(c *gin.Context) SearchFilters {
	filters := SearchFilters{}

	// Parse types filter (comma-separated)
	if types := c.Query("types"); types != "" {
		filters.Types = strings.Split(types, ",")
	}

	// Parse categories filter (comma-separated)
	if categories := c.Query("categories"); categories != "" {
		filters.Categories = strings.Split(categories, ",")
	}

	// Parse price range
	if minPrice := c.Query("min_price"); minPrice != "" {
		filters.MinPrice, _ = strconv.ParseFloat(minPrice, 64)
	}
	if maxPrice := c.Query("max_price"); maxPrice != "" {
		filters.MaxPrice, _ = strconv.ParseFloat(maxPrice, 64)
	}

	// Parse market cap range
	if minCap := c.Query("min_market_cap"); minCap != "" {
		filters.MinMarketCap, _ = strconv.ParseFloat(minCap, 64)
	}
	if maxCap := c.Query("max_market_cap"); maxCap != "" {
		filters.MaxMarketCap, _ = strconv.ParseFloat(maxCap, 64)
	}

	// Parse active status
	if active := c.Query("is_active"); active != "" {
		isActive := active == "true"
		filters.IsActive = &isActive
	}

	return filters
}

// parseSortOptions parses sorting parameters from the request
func parseSortOptions(c *gin.Context) SortOptions {
	validFields := map[string]bool{
		"symbol": true, "name": true, "type": true, "category": true,
		"market_cap": true, "price": true, "volume_24h": true,
		"change_24h": true, "rank": true, "listed_since": true,
	}
	s := apiutil.ParseSort(c, "rank", "asc", validFields)
	return SortOptions{
		Field:     s.Field,
		Direction: s.Direction,
	}
}

// matchesFilters checks if a symbol matches the given filters
func matchesFilters(symbol SymbolInfo, filters SearchFilters) bool {
	// Type filter
	if len(filters.Types) > 0 {
		found := false
		for _, t := range filters.Types {
			if strings.EqualFold(symbol.Type, t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Category filter
	if len(filters.Categories) > 0 {
		found := false
		for _, c := range filters.Categories {
			if strings.EqualFold(symbol.Category, c) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Price range filter
	if filters.MinPrice > 0 && symbol.Price < filters.MinPrice {
		return false
	}
	if filters.MaxPrice > 0 && symbol.Price > filters.MaxPrice {
		return false
	}

	// Market cap range filter
	if filters.MinMarketCap > 0 && symbol.MarketCap < filters.MinMarketCap {
		return false
	}
	if filters.MaxMarketCap > 0 && symbol.MarketCap > filters.MaxMarketCap {
		return false
	}

	// Active status filter
	if filters.IsActive != nil && symbol.IsActive != *filters.IsActive {
		return false
	}

	return true
}

// sortSymbols sorts the symbols based on the given options
func sortSymbols(symbols []SymbolInfo, sort SortOptions) {
	less := func(i, j int) bool {
		var result bool
		switch sort.Field {
		case "symbol":
			result = symbols[i].Symbol < symbols[j].Symbol
		case "name":
			result = symbols[i].Name < symbols[j].Name
		case "type":
			result = symbols[i].Type < symbols[j].Type
		case "category":
			result = symbols[i].Category < symbols[j].Category
		case "market_cap":
			result = symbols[i].MarketCap < symbols[j].MarketCap
		case "price":
			result = symbols[i].Price < symbols[j].Price
		case "volume_24h":
			result = symbols[i].Volume24h < symbols[j].Volume24h
		case "change_24h":
			result = symbols[i].Change24h < symbols[j].Change24h
		case "rank":
			result = symbols[i].Rank < symbols[j].Rank
		case "listed_since":
			result = symbols[i].ListedSince < symbols[j].ListedSince
		default:
			result = symbols[i].Rank < symbols[j].Rank
		}

		if sort.Direction == "desc" {
			return !result
		}
		return result
	}

	// Simple bubble sort for simplicity (for larger datasets, use sort.Slice)
	n := len(symbols)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if !less(j, j+1) {
				symbols[j], symbols[j+1] = symbols[j+1], symbols[j]
			}
		}
	}
}

// advancedSearchHandler handles advanced symbol search with filters and sorting
func advancedSearchHandler(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	qf := logging.QueryLogFields(query)
	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithField(logging.FieldEvent, "advanced_search_request").Debug("advanced search request received")

	// Parse pagination (reused primitive)
	pagination := apiutil.ParsePagination(c, 20, 100)

	// Parse filters and sort options
	filters := parseSearchFilters(c)
	sort := parseSortOptions(c)

	// Search and filter symbols
	symbolDBMutex.RLock()
	var results []SymbolInfo
	for _, symbol := range symbolDatabase {
		// Text search on symbol or name
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(symbol.Symbol), queryLower) &&
				!strings.Contains(strings.ToLower(symbol.Name), queryLower) {
				continue
			}
		}

		// Apply filters
		if matchesFilters(symbol, filters) {
			results = append(results, symbol)
		}
	}
	symbolDBMutex.RUnlock()

	// Sort results
	sortSymbols(results, sort)

	// Pagination
	total := len(results)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginatedResults := results[start:end]

	// Get available filter options
	availableTypes := make(map[string]bool)
	availableCategories := make(map[string]bool)
	symbolDBMutex.RLock()
	for _, symbol := range symbolDatabase {
		availableTypes[symbol.Type] = true
		availableCategories[symbol.Category] = true
	}
	symbolDBMutex.RUnlock()

	typeList := make([]string, 0, len(availableTypes))
	for t := range availableTypes {
		typeList = append(typeList, t)
	}

	categoryList := make([]string, 0, len(availableCategories))
	for c := range availableCategories {
		categoryList = append(categoryList, c)
	}

	logging.WithComponent(logging.ComponentSearch).WithFields(qf).WithFields(log.Fields{
		logging.FieldEvent: "advanced_search_completed",
		"result_count":     len(paginatedResults),
		"total":            total,
		"page":             pagination.Page,
		"sort_by":          sort.Field,
		"sort_dir":         sort.Direction,
	}).Info("advanced search completed")

	c.JSON(http.StatusOK, gin.H{
		"data": paginatedResults,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"filters_applied": gin.H{
			"types":          filters.Types,
			"categories":     filters.Categories,
			"min_price":      filters.MinPrice,
			"max_price":      filters.MaxPrice,
			"min_market_cap": filters.MinMarketCap,
			"max_market_cap": filters.MaxMarketCap,
		},
		"sort_applied": gin.H{
			"field":     sort.Field,
			"direction": sort.Direction,
		},
		"available_filters": gin.H{
			"types":      typeList,
			"categories": categoryList,
		},
	})
}

// exportAdvancedSearchHandler returns advanced symbol search results as machine-friendly JSON for archival or analysis.
// Same parameters as GET /api/search/advanced (q, types, categories, min_price, max_price, page, page_size, sort_by, sort_dir).
// Public endpoint; no auth required.
func exportAdvancedSearchHandler(c *gin.Context) {
	if !checkExportRateLimit(c, false) {
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	pagination := apiutil.ParsePagination(c, 20, 100)
	filters := parseSearchFilters(c)
	sort := parseSortOptions(c)

	symbolDBMutex.RLock()
	var results []SymbolInfo
	for _, symbol := range symbolDatabase {
		if query != "" {
			queryLower := strings.ToLower(query)
			if !strings.Contains(strings.ToLower(symbol.Symbol), queryLower) &&
				!strings.Contains(strings.ToLower(symbol.Name), queryLower) {
				continue
			}
		}
		if matchesFilters(symbol, filters) {
			results = append(results, symbol)
		}
	}
	symbolDBMutex.RUnlock()

	sortSymbols(results, sort)

	total := len(results)
	start := pagination.Offset
	end := start + pagination.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	paginatedResults := results[start:end]
	if total >= 50 || pagination.PageSize >= 50 {
		logLargeExport(c, "search/advanced/export", map[string]interface{}{"total": total, "page_size": pagination.PageSize, "query": query})
	}
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusOK, gin.H{
		"export_meta": gin.H{
			"export_timestamp": time.Now().UTC().Format(time.RFC3339),
			"export_version":   export.Version,
			"endpoint":         "search/advanced",
			"query":            query,
			"filters_applied": gin.H{
				"types":          filters.Types,
				"categories":     filters.Categories,
				"min_price":      filters.MinPrice,
				"max_price":      filters.MaxPrice,
				"min_market_cap": filters.MinMarketCap,
				"max_market_cap": filters.MaxMarketCap,
			},
			"sort_applied": gin.H{
				"field":     sort.Field,
				"direction": sort.Direction,
			},
		},
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
		"data": paginatedResults,
	})
}

// getSymbolCategoriesHandler returns available symbol categories
func getSymbolCategoriesHandler(c *gin.Context) {
	categories := map[string][]string{
		"types":      {"crypto", "stock", "commodity", "forex"},
		"categories": {"layer1", "layer2", "defi", "nft", "stablecoin", "payment", "exchange", "meme", "privacy", "infrastructure"},
	}
	c.JSON(http.StatusOK, categories)
}

func metricsHandler(c *gin.Context) {
	if rdb == nil {
		c.JSON(http.StatusOK, gin.H{"mempool_size": []map[string]interface{}{}, "block_times": []map[string]interface{}{}, "tx_volume": []map[string]interface{}{}})
		return
	}
	mempoolData, _ := rdb.ZRangeWithScores(ctx, "mempool_size", -100, -1).Result()
	blockTimeData, _ := rdb.ZRangeWithScores(ctx, "block_times", -100, -1).Result()
	txVolumeData, _ := rdb.ZRangeWithScores(ctx, "tx_volume", -100, -1).Result()

	// Convert to chart-friendly format
	mempool := []map[string]interface{}{}
	for _, z := range mempoolData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		mempool = append(mempool, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	blockTimes := []map[string]interface{}{}
	for _, z := range blockTimeData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		blockTimes = append(blockTimes, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	txVolumes := []map[string]interface{}{}
	for _, z := range txVolumeData {
		timestamp := int64(z.Score)
		value := z.Member.(string)
		txVolumes = append(txVolumes, map[string]interface{}{
			"time":  timestamp,
			"value": value,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"mempool_size": mempool,
		"block_times":  blockTimes,
		"tx_volume":    txVolumes,
	})
}

func networkStatusHandler(c *gin.Context) {
	data, err := explorerSvc.GetNetworkStatus(c.Request.Context())
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, data)
}

// Default and max points for price history (5-min interval: 288 = 24h, 8640 = 30d).
const priceHistoryDefaultPoints = 288
const priceHistoryMaxPoints = 8640

func priceHistoryHandler(c *gin.Context) {
	if rdb == nil {
		handleError(c, errors.New("redis not available"), http.StatusInternalServerError)
		return
	}

	currency := strings.ToLower(strings.TrimSpace(c.DefaultQuery("currency", "usd")))
	if !pricing.SupportedFiatCurrencies[currency] {
		currency = "usd"
	}
	limit := priceHistoryDefaultPoints
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			if n > priceHistoryMaxPoints {
				n = priceHistoryMaxPoints
			}
			limit = n
		}
	}

	key := btcPriceHistoryKey(currency)
	if currency == "usd" {
		// Prefer legacy key if per-currency key is empty (backfill)
		n, _ := rdb.ZCard(ctx, key).Result()
		if n == 0 {
			key = "btc_price_history"
		}
	}

	history, err := rdb.ZRevRangeWithScores(ctx, key, 0, int64(limit-1)).Result()
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	type PricePoint struct {
		Timestamp int64   `json:"timestamp"`
		Price     float64 `json:"price"`
	}

	result := make([]PricePoint, 0, len(history))
	for _, z := range history {
		priceStr, ok := z.Member.(string)
		if !ok {
			continue
		}
		price, _ := strconv.ParseFloat(priceStr, 64)
		result = append(result, PricePoint{
			Timestamp: int64(z.Score),
			Price:     price,
		})
	}

	c.Header("X-Price-History-Currency", currency)
	c.JSON(http.StatusOK, result)
}

// Redis key for BTC multi-currency rates. TTL set by RATES_CACHE_TTL_SECONDS (default 60s).
const ratesRedisKeyBTC = "rates:btc"

// ratesCacheTTL returns the TTL for rates cache from config or default 60s.
func ratesCacheTTL() time.Duration {
	if appConfig != nil && appConfig.RatesCacheTTLSeconds > 0 {
		return time.Duration(appConfig.RatesCacheTTLSeconds) * time.Second
	}
	return 60 * time.Second
}

func ratesHandler(c *gin.Context) {
	ctx := context.Background()

	// Optional: filter by user-requested fiat currencies (comma-separated, e.g. currency=usd,eur)
	var wantCurrencies []string
	if q := strings.TrimSpace(c.Query("currency")); q != "" {
		for _, code := range strings.Split(q, ",") {
			code = strings.ToLower(strings.TrimSpace(code))
			if code != "" && pricing.SupportedFiatCurrencies[code] {
				wantCurrencies = append(wantCurrencies, code)
			}
		}
	}

	// Try cache first (we store the default set under rates:btc)
	if rdb != nil {
		cached, err := rdb.Get(ctx, ratesRedisKeyBTC).Result()
		if err == nil && cached != "" {
			var data map[string]interface{}
			if unmarshalErr := json.Unmarshal([]byte(cached), &data); unmarshalErr == nil {
				metrics.RecordRates(true)
				rates := filterRatesByCurrencies(data, wantCurrencies)
				c.JSON(http.StatusOK, rates)
				return
			}
		}
	}
	metrics.RecordRates(false)

	if pricingClient == nil {
		handleError(c, errors.New("pricing client not initialized"), http.StatusInternalServerError)
		return
	}

	// Always fetch the default currency set so we can cache one canonical entry under rates:btc
	rates, err := pricingClient.GetMultiCurrencyRatesIn(ctx, nil)
	if err != nil {
		handleError(c, err, http.StatusInternalServerError)
		return
	}

	// Store in Redis with clear key naming and configured TTL
	if rdb != nil {
		if ratesJSON, err := json.Marshal(rates); err == nil {
			_ = rdb.Set(ctx, ratesRedisKeyBTC, ratesJSON, ratesCacheTTL()).Err()
		}
	}

	out := filterRatesByCurrencies(rates, wantCurrencies)
	ttl := ratesCacheTTL()
	c.Header("X-Rates-Cache-TTL-Seconds", strconv.Itoa(int(ttl.Seconds())))
	c.Header("X-Rates-Updated-At", time.Now().UTC().Format(time.RFC3339))
	c.JSON(http.StatusOK, out)
}

// filterRatesByCurrencies returns a copy of the rates map containing only the requested currencies.
// CoinGecko response shape: {"bitcoin": {"usd": 123, "eur": 100}}. If want is empty, return as-is.
func filterRatesByCurrencies(rates map[string]interface{}, want []string) map[string]interface{} {
	if len(want) == 0 {
		return rates
	}
	wantSet := make(map[string]bool)
	for _, c := range want {
		wantSet[c] = true
	}
	out := make(map[string]interface{})
	for asset, v := range rates {
		vm, ok := v.(map[string]interface{})
		if !ok {
			out[asset] = v
			continue
		}
		filtered := make(map[string]interface{})
		for k, val := range vm {
			if wantSet[k] {
				filtered[k] = val
			}
		}
		if len(filtered) > 0 {
			out[asset] = filtered
		}
	}
	return out
}

// getUSDPerFiat returns how many USD equal 1 unit of fiat (e.g. 1 EUR = 1.08 USD).
// Derived from BTC/USD and BTC/fiat; used to convert commodity/bond USD prices into user fiat.
