package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/repos"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func feedbackHandler(c *gin.Context) {
	var feedbackReq struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Message string `json:"message" binding:"required"`
	}

	if err := c.ShouldBindJSON(&feedbackReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	feedbackReq.Name = strings.TrimSpace(feedbackReq.Name)
	feedbackReq.Email = strings.TrimSpace(feedbackReq.Email)
	feedbackReq.Message = strings.TrimSpace(feedbackReq.Message)

	if len(feedbackReq.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_name", "Name must be at most 100 characters")
		return
	}
	if feedbackReq.Email != "" {
		if len(feedbackReq.Email) > 254 {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Email must be at most 254 characters")
			return
		}
		emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
		if !emailPattern.MatchString(feedbackReq.Email) {
			errorResponse(c, http.StatusBadRequest, "invalid_email", "Invalid email format")
			return
		}
	}
	if len(feedbackReq.Message) < 5 || len(feedbackReq.Message) > 1000 {
		errorResponse(c, http.StatusBadRequest, "invalid_message", "Message must be between 5 and 1000 characters")
		return
	}

	err := feedbackSvc.Store(ctx, sanitizeText(feedbackReq.Name, 100), feedbackReq.Email, sanitizeText(feedbackReq.Message, 1000), c.ClientIP())
	if err != nil {
		logging.WithComponent(logging.ComponentFeedback).WithError(err).WithField(logging.FieldEvent, "redis_set_failed").Error("failed to store feedback in Redis")
		errorResponse(c, http.StatusInternalServerError, "feedback_save_failed", "Failed to save feedback")
		return
	}

	logging.WithComponent(logging.ComponentFeedback).WithFields(log.Fields{
		logging.FieldEvent: "feedback_stored",
		"message_len":      len(feedbackReq.Message),
		"has_email":        feedbackReq.Email != "",
		"has_name":         feedbackReq.Name != "",
	}).Info("feedback stored")

	c.JSON(http.StatusOK, gin.H{"message": "Thank you for your feedback!"})
}

// adminStatusHandler provides system status for admin
func adminStatusHandler(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")

	// Get Redis info
	info := adminSvc.RedisMemoryInfo(ctx)

	// Get rate limiting stats
	activeLimits := adminSvc.ActiveRateLimitEntries(ctx)

	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"user":               username,
		"role":               role,
		"redis_memory":       info,
		"active_rate_limits": activeLimits,
		"timestamp":          time.Now().Unix(),
	})
}

// getActiveRateLimitCount returns the number of active rate limit entries.
// When Redis is available, it counts keys with the "rate:" prefix; otherwise,
// it falls back to the in-memory map size.
func getActiveRateLimitCount() int {
	if rdb != nil {
		ctx := context.Background()
		iter := rdb.Scan(ctx, 0, "rate:*", 0).Iterator()
		count := 0
		for iter.Next(ctx) {
			count++
		}
		if err := iter.Err(); err != nil {
			logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_scan_failed").Warn("failed to scan rate limit keys from Redis")
		}
		return count
	}

	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()
	return len(rateLimitCount)
}

// adminCacheHandler provides cache management for admin
func adminCacheHandler(c *gin.Context) {
	action := c.Query("action")
	username, _ := c.Get("username")

	switch action {
	case "clear":
		// Clear all cache keys
		keys, err := adminSvc.ListCacheKeys(ctx)
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "cache_keys_failed", "Failed to get cache keys")
			return
		}

		if err := adminSvc.DeleteCacheKeys(ctx, keys); err != nil {
			errorResponse(c, http.StatusInternalServerError, "cache_keys_failed", "Failed to delete cache keys")
			return
		}

		logging.WithComponent(logging.ComponentAdmin).WithFields(log.Fields{
			logging.FieldUsername: username,
			logging.FieldEvent:    "cache_cleared",
		}).Info("cache cleared by admin")
		c.JSON(http.StatusOK, gin.H{"message": "Cache cleared successfully", "keys_removed": len(keys)})

	case "stats":
		keys, err := adminSvc.ListCacheKeys(ctx)
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "cache_stats_failed", "Failed to get cache stats")
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"total_keys": len(keys),
			"keys":       keys,
		})

	default:
		errorResponse(c, http.StatusBadRequest, "invalid_action", "Invalid action. Use 'clear' or 'stats'")
	}
}

// Portfolio management handlers

// listPortfoliosHandler returns all portfolios for the authenticated user
func listPortfoliosHandler(c *gin.Context) {
	username, _ := c.Get("username")

	portfolios, err := portfolioSvc.ListPortfolios(ctx, username.(string))
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to fetch portfolios")
		return
	}

	// Apply sorting on created or updated timestamps
	sortParams := apiutil.ParseSort(c, "created", "desc", map[string]bool{
		"created": true,
		"updated": true,
	})

	switch sortParams.Field {
	case "updated":
		// sort by Updated desc/asc
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
		// sort by Created desc/asc
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

	// Apply pagination using shared primitive
	pagination := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSize)
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

	// Valuation currency: optional query override, else user's preferred fiat (fallback to usd)
	valuationCurrency := "usd"
	if q := strings.ToLower(strings.TrimSpace(c.Query("currency"))); q != "" && pricing.SupportedFiatCurrencies[q] {
		valuationCurrency = q
	} else if u, ok := getUser(username.(string)); ok && u.PreferredCurrency != "" {
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

	c.JSON(http.StatusOK, gin.H{
		"data": dataWithValuation,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
	})
}

// createPortfolioHandler creates a new portfolio
func createPortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")

	var p Portfolio
	if err := c.ShouldBindJSON(&p); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	if p.Name == "" || len(p.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_name", "Portfolio name must be between 1 and 100 characters")
		return
	}
	if len(p.Description) > 500 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_description", "Portfolio description must be at most 500 characters")
		return
	}
	if len(p.Items) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_items", "Portfolio cannot contain more than 100 items")
		return
	}
	for i, item := range p.Items {
		item.Type = strings.TrimSpace(item.Type)
		item.Label = strings.TrimSpace(item.Label)
		item.Address = strings.TrimSpace(item.Address)
		if item.Label == "" || len(item.Label) > 100 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_label", fmt.Sprintf("Item %d label must be between 1 and 100 characters", i+1))
			return
		}
		if item.Address == "" || len(item.Address) > 256 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_address", fmt.Sprintf("Item %d address must be between 1 and 256 characters", i+1))
			return
		}
		switch strings.ToLower(item.Type) {
		case "stock", "crypto", "bond", "commodity":
			// allowed
		default:
			errorResponse(c, http.StatusBadRequest, "invalid_item_type", fmt.Sprintf("Item %d has invalid type", i+1))
			return
		}
		item.Label = sanitizeText(item.Label, 100)
		// Addresses are identifiers; normalize whitespace and strip control chars without HTML-escaping.
		item.Address = sanitizeText(item.Address, 256)
		p.Items[i] = item
	}

	p.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	p.Username = username.(string)
	p.Name = sanitizeText(p.Name, 100)
	p.Description = sanitizeText(p.Description, 500)
	p.Created = time.Now()
	p.Updated = time.Now()

	if err := portfolioSvc.SavePortfolio(ctx, &p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_save_failed", "Failed to save portfolio")
		return
	}

	c.JSON(http.StatusCreated, p)
}

// updatePortfolioHandler updates an existing portfolio
func updatePortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")
	portfolioID := c.Param("id")

	var updateReq Portfolio
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}

	updateReq.Name = strings.TrimSpace(updateReq.Name)
	updateReq.Description = strings.TrimSpace(updateReq.Description)
	if updateReq.Name == "" || len(updateReq.Name) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_name", "Portfolio name must be between 1 and 100 characters")
		return
	}
	if len(updateReq.Description) > 500 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_description", "Portfolio description must be at most 500 characters")
		return
	}
	if len(updateReq.Items) > 100 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_items", "Portfolio cannot contain more than 100 items")
		return
	}
	for i, item := range updateReq.Items {
		item.Type = strings.TrimSpace(item.Type)
		item.Label = strings.TrimSpace(item.Label)
		item.Address = strings.TrimSpace(item.Address)
		if item.Label == "" || len(item.Label) > 100 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_label", fmt.Sprintf("Item %d label must be between 1 and 100 characters", i+1))
			return
		}
		if item.Address == "" || len(item.Address) > 256 {
			errorResponse(c, http.StatusBadRequest, "invalid_item_address", fmt.Sprintf("Item %d address must be between 1 and 256 characters", i+1))
			return
		}
		switch strings.ToLower(item.Type) {
		case "stock", "crypto", "bond", "commodity":
			// allowed
		default:
			errorResponse(c, http.StatusBadRequest, "invalid_item_type", fmt.Sprintf("Item %d has invalid type", i+1))
			return
		}
		item.Label = sanitizeText(item.Label, 100)
		item.Address = sanitizeText(item.Address, 256)
		updateReq.Items[i] = item
	}

	p, err := portfolioSvc.GetPortfolio(ctx, username.(string), portfolioID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "portfolio_decode_failed", "Failed to decode portfolio")
		}
		return
	}

	// Update fields
	p.Name = sanitizeText(updateReq.Name, 100)
	p.Description = sanitizeText(updateReq.Description, 500)
	p.Items = updateReq.Items
	p.Updated = time.Now()

	if err := portfolioSvc.SavePortfolio(ctx, p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_save_failed", "Failed to save portfolio")
		return
	}

	c.JSON(http.StatusOK, p)
}

// deletePortfolioHandler deletes a portfolio
func deletePortfolioHandler(c *gin.Context) {
	username, _ := c.Get("username")
	portfolioID := c.Param("id")

	if err := portfolioSvc.DeletePortfolio(ctx, username.(string), portfolioID); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_delete_failed", "Failed to delete portfolio")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Portfolio deleted successfully"})
}

// watchlistKey delegates to repos.WatchlistKey (single source of truth for Redis keys).
func watchlistKey(username, id string) string {
	return repos.WatchlistKey(username, id)
}

const (
	maxWatchlistsPerUser = 20 // quota: max watchlists per user to avoid unbounded storage
	maxWatchlistEntries  = 100
	maxWatchlistNameLen  = 100
	maxEntrySymbolLen    = 128
	maxEntryAddressLen   = 256
	maxEntryNotesLen     = 500
	maxEntryTags         = 10
	maxTagLen            = 50
	maxEntryGroupLen     = 50
)

func validateWatchlistEntry(i int, e *WatchlistEntry) error {
	e.Type = strings.ToLower(strings.TrimSpace(e.Type))
	if e.Type != "symbol" && e.Type != "address" {
		return fmt.Errorf("entry %d: type must be symbol or address", i+1)
	}
	e.Symbol = strings.TrimSpace(e.Symbol)
	e.Address = strings.TrimSpace(e.Address)
	if e.Type == "symbol" {
		if e.Symbol == "" || len(e.Symbol) > maxEntrySymbolLen {
			return fmt.Errorf("entry %d: symbol must be 1-%d characters", i+1, maxEntrySymbolLen)
		}
		e.Symbol = sanitizeText(e.Symbol, maxEntrySymbolLen)
	} else {
		if e.Address == "" || len(e.Address) > maxEntryAddressLen {
			return fmt.Errorf("entry %d: address must be 1-%d characters", i+1, maxEntryAddressLen)
		}
		e.Address = sanitizeText(e.Address, maxEntryAddressLen)
	}
	e.Notes = sanitizeText(strings.TrimSpace(e.Notes), maxEntryNotesLen)
	if len(e.Tags) > maxEntryTags {
		return fmt.Errorf("entry %d: at most %d tags allowed", i+1, maxEntryTags)
	}
	for j, t := range e.Tags {
		t = strings.TrimSpace(t)
		if len(t) > maxTagLen {
			return fmt.Errorf("entry %d tag %d: tag max %d characters", i+1, j+1, maxTagLen)
		}
		e.Tags[j] = sanitizeText(t, maxTagLen)
	}
	e.Group = strings.TrimSpace(e.Group)
	if len(e.Group) > maxEntryGroupLen {
		return fmt.Errorf("entry %d: group must be at most %d characters", i+1, maxEntryGroupLen)
	}
	e.Group = sanitizeText(e.Group, maxEntryGroupLen)
	return nil
}

// listWatchlistsHandler returns all watchlists for the authenticated user.
func listWatchlistsHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	watchlists, err := watchlistSvc.ListWatchlists(ctx, uname)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to fetch watchlists")
		return
	}
	sort.Slice(watchlists, func(i, j int) bool {
		return watchlists[i].Updated.After(watchlists[j].Updated)
	})
	pagination := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSize)
	total := len(watchlists)
	start := pagination.Offset
	if start > total {
		start = total
	}
	end := start + pagination.PageSize
	if end > total {
		end = total
	}
	pageSlice := watchlists[start:end]
	c.JSON(http.StatusOK, gin.H{
		"data": pageSlice,
		"pagination": gin.H{
			"page":        pagination.Page,
			"page_size":   pagination.PageSize,
			"total":       total,
			"total_pages": (total + pagination.PageSize - 1) / pagination.PageSize,
		},
	})
}

// createWatchlistHandler creates a new watchlist. Enforces per-user watchlist quota.
func createWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	count, err := watchlistSvc.CountWatchlists(ctx, uname)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to check watchlist count")
		return
	}
	if count >= maxWatchlistsPerUser {
		errorResponse(c, http.StatusTooManyRequests, "watchlist_quota_exceeded", fmt.Sprintf("Maximum %d watchlists per user", maxWatchlistsPerUser))
		return
	}
	var w Watchlist
	if err := c.ShouldBindJSON(&w); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	w.Name = strings.TrimSpace(w.Name)
	if len(w.Name) > maxWatchlistNameLen {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_name", "Watchlist name must be at most 100 characters")
		return
	}
	w.Name = sanitizeText(w.Name, maxWatchlistNameLen)
	if len(w.Entries) > maxWatchlistEntries {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_entries", "Watchlist cannot contain more than 100 entries")
		return
	}
	for i := range w.Entries {
		if err := validateWatchlistEntry(i, &w.Entries[i]); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
			return
		}
	}
	w.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	w.Username = uname
	w.Created = time.Now()
	w.Updated = time.Now()
	if err := watchlistSvc.SaveWatchlist(ctx, &w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusCreated, w)
}

// getWatchlistHandler returns a single watchlist by ID.
func getWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
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
	c.JSON(http.StatusOK, w)
}

// updateWatchlistHandler updates an existing watchlist. Enforces entry count quota.
func updateWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	var w Watchlist
	if err := c.ShouldBindJSON(&w); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request format")
		return
	}
	w.Name = strings.TrimSpace(w.Name)
	if len(w.Name) > maxWatchlistNameLen {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_name", "Watchlist name must be at most 100 characters")
		return
	}
	w.Name = sanitizeText(w.Name, maxWatchlistNameLen)
	if len(w.Entries) > maxWatchlistEntries {
		errorResponse(c, http.StatusBadRequest, "invalid_watchlist_entries", "Watchlist cannot contain more than 100 entries")
		return
	}
	for i := range w.Entries {
		if err := validateWatchlistEntry(i, &w.Entries[i]); err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_entry", err.Error())
			return
		}
	}
	existing, err := watchlistSvc.GetWatchlist(ctx, uname, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			errorResponse(c, http.StatusNotFound, "watchlist_not_found", "Watchlist not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "watchlist_fetch_failed", "Failed to load watchlist")
		}
		return
	}
	existing.Name = w.Name
	existing.Entries = w.Entries
	existing.Updated = time.Now()
	if err := watchlistSvc.SaveWatchlist(ctx, existing); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusOK, existing)
}

// deleteWatchlistHandler deletes a watchlist.
func deleteWatchlistHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Watchlists require Redis")
		return
	}
	if err := watchlistSvc.DeleteWatchlist(ctx, uname, id); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_delete_failed", "Failed to delete watchlist")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Watchlist deleted successfully"})
}

// addWatchlistEntryHandler appends one entry to a watchlist. Enforces per-watchlist entry quota.
func addWatchlistEntryHandler(c *gin.Context) {
	username, ok := c.Get("username")
	if !ok {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}
	uname := username.(string)
	id := c.Param("id")
	if id == "" {
		errorResponse(c, http.StatusBadRequest, "invalid_id", "Watchlist ID required")
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
	if len(w.Entries) >= maxWatchlistEntries {
		errorResponse(c, http.StatusTooManyRequests, "entry_quota_exceeded", fmt.Sprintf("Maximum %d entries per watchlist", maxWatchlistEntries))
		return
	}
	w.Entries = append(w.Entries, e)
	w.Updated = time.Now()
	if err := watchlistSvc.SaveWatchlist(ctx, w); err != nil {
		errorResponse(c, http.StatusInternalServerError, "watchlist_save_failed", "Failed to save watchlist")
		return
	}
	c.JSON(http.StatusCreated, w)
}

// updateWatchlistEntryHandler updates an entry at the given 0-based index.
