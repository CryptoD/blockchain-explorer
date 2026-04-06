package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/logging"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/CryptoD/blockchain-explorer/internal/news"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
)

// In-memory fallback for METRICS_RATE_LIMIT_PER_IP when Redis is unavailable.
var (
	metricsRLCount = make(map[string]int)
	metricsRLReset = make(map[string]time.Time)
	metricsRLMutex sync.Mutex
)

func updateProfileHandler(c *gin.Context) {
	usernameVal, exists := c.Get("username")
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found in session")
		return
	}
	username := usernameVal.(string)

	var body updateProfileRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	user, exists := getUser(username)
	if !exists {
		errorResponse(c, http.StatusNotFound, "user_profile_not_found", "User profile not found")
		return
	}

	if body.Email != nil {
		v := strings.TrimSpace(*body.Email)
		if v != "" {
			if len(v) > 254 {
				errorResponse(c, http.StatusBadRequest, "invalid_email", "Email must be at most 254 characters")
				return
			}
			emailPattern := regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
			if !emailPattern.MatchString(v) {
				errorResponse(c, http.StatusBadRequest, "invalid_email", "Invalid email format")
				return
			}
		}
		user.Email = v
	}

	if body.PreferredCurrency != nil {
		code := strings.ToLower(strings.TrimSpace(*body.PreferredCurrency))
		if code != "" && !pricing.SupportedFiatCurrencies[code] {
			errorResponse(c, http.StatusBadRequest, "unsupported_currency", "Unsupported currency code; use a supported fiat code (e.g. usd, eur, gbp)")
			return
		}
		user.PreferredCurrency = code
	}
	if body.Theme != nil {
		v := strings.ToLower(strings.TrimSpace(*body.Theme))
		if v != "" && !supportedThemes[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_theme", "Theme must be one of: light, dark, system")
			return
		}
		user.Theme = v
	}
	if body.Language != nil {
		v := strings.ToLower(strings.TrimSpace(*body.Language))
		if v != "" && !supportedLangs[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_language", "Unsupported language code; use a supported code (e.g. en, es)")
			return
		}
		user.Language = v
	}
	if body.NotificationsEmail != nil {
		user.NotificationsEmail = *body.NotificationsEmail
	}
	if body.NotificationsPriceAlerts != nil {
		user.NotificationsPriceAlerts = *body.NotificationsPriceAlerts
	}
	if body.EmailPriceAlerts != nil {
		user.EmailPriceAlerts = *body.EmailPriceAlerts
	}
	if body.EmailPortfolioEvents != nil {
		user.EmailPortfolioEvents = *body.EmailPortfolioEvents
	}
	if body.EmailProductUpdates != nil {
		user.EmailProductUpdates = *body.EmailProductUpdates
	}
	if body.DefaultLandingPage != nil {
		v := strings.ToLower(strings.TrimSpace(*body.DefaultLandingPage))
		if v != "" && !supportedLandingPages[v] {
			errorResponse(c, http.StatusBadRequest, "invalid_default_landing_page", "Default landing page must be one of: explorer, dashboard, portfolios")
			return
		}
		user.DefaultLandingPage = v
	}

	if body.NewsSourcesFavorite != nil {
		list, err := normalizeNewsSourceList(*body.NewsSourcesFavorite, 50)
		if err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_news_sources_favorite", err.Error())
			return
		}
		user.NewsSourcesFavorite = list
	}
	if body.NewsSourcesBlocked != nil {
		list, err := normalizeNewsSourceList(*body.NewsSourcesBlocked, 200)
		if err != nil {
			errorResponse(c, http.StatusBadRequest, "invalid_news_sources_blocked", err.Error())
			return
		}
		user.NewsSourcesBlocked = list
	}

	userMutex.Lock()
	users[username] = user
	userMutex.Unlock()

	if err := saveUserToRedis(user); err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to persist profile to Redis")
		errorResponse(c, http.StatusInternalServerError, "save_failed", "Failed to save profile")
		return
	}

	c.JSON(http.StatusOK, user)
}

// changePasswordHandler updates the authenticated user's password and rotates the CSRF token for the
// current session so any previously issued X-CSRF-Token value stops working (roadmap task 31).
func changePasswordHandler(c *gin.Context) {
	usernameVal, exists := c.Get("username")
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found in session")
		return
	}
	username := usernameVal.(string)

	sessionID, err := c.Cookie("session_id")
	if err != nil || strings.TrimSpace(sessionID) == "" {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		return
	}

	var body struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if !isStrongPassword(body.NewPassword) {
		errorResponse(c, http.StatusBadRequest, "invalid_password", "Password must be 8-128 characters and include at least one letter and one digit")
		return
	}

	user, ok := getUser(username)
	if !ok {
		errorResponse(c, http.StatusNotFound, "user_profile_not_found", "User profile not found")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.CurrentPassword)); err != nil {
		errorResponse(c, http.StatusUnauthorized, "invalid_credentials", "Current password is incorrect")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Error("password hash failed")
		errorResponse(c, http.StatusInternalServerError, "password_update_failed", "Failed to update password")
		return
	}
	user.Password = string(hashed)
	userMutex.Lock()
	users[username] = user
	userMutex.Unlock()

	if err := saveUserToRedis(user); err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldUsername, username).Warn("Failed to persist password change to Redis")
		errorResponse(c, http.StatusInternalServerError, "save_failed", "Failed to save profile")
		return
	}

	newCSRF, err := authSvc.CreateOrUpdateCSRFToken(sessionID)
	if err != nil {
		logging.WithComponent(logging.ComponentAuth).WithError(err).WithField(logging.FieldEvent, "csrf_rotate_failed").Error("failed to rotate CSRF after password change")
		errorResponse(c, http.StatusInternalServerError, "csrf_rotation_failed", "Failed to rotate CSRF token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Password updated",
		"csrfToken": newCSRF,
	})
}

// authMiddleware checks for valid authentication
func authMiddleware(c *gin.Context) {
	sessionID, err := c.Cookie("session_id")
	if err != nil {
		errorResponse(c, http.StatusUnauthorized, "authentication_required", "Authentication required")
		c.Abort()
		return
	}

	username, valid := validateSession(sessionID)
	if !valid {
		errorResponse(c, http.StatusUnauthorized, "invalid_session", "Invalid session")
		c.Abort()
		return
	}

	user, exists := getUser(username)
	if !exists {
		errorResponse(c, http.StatusUnauthorized, "user_not_found", "User not found")
		c.Abort()
		return
	}

	c.Set("username", username)
	c.Set("role", user.Role)
	c.Next()
}

// requireRoleMiddleware checks for specific role access
func requireRoleMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			errorResponse(c, http.StatusForbidden, "role_missing", "Role information missing")
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok {
			errorResponse(c, http.StatusForbidden, "role_invalid", "Invalid role format")
			c.Abort()
			return
		}

		if userRole != requiredRole && userRole != "admin" { // Admin can access everything
			errorResponse(c, http.StatusForbidden, "insufficient_permissions", "Insufficient permissions")
			c.Abort()
			return
		}

		c.Next()
	}
}

// enforceMetricsUnauthenticatedRateLimit applies a separate per-IP budget for GET /metrics when
// metrics are enabled without METRICS_TOKEN (see docs/RATE_LIMITS.md). Returns true if the request was aborted with 429.
func enforceMetricsUnauthenticatedRateLimit(c *gin.Context) bool {
	if appConfig == nil || !appConfig.MetricsEnabled || strings.TrimSpace(appConfig.MetricsToken) != "" {
		return false
	}
	limit := appConfig.MetricsRateLimitPerIP
	if limit <= 0 {
		return false
	}
	windowSeconds := 60
	if appConfig.RateLimitWindowSeconds > 0 {
		windowSeconds = appConfig.RateLimitWindowSeconds
	}
	window := time.Duration(windowSeconds) * time.Second
	ip := c.ClientIP()
	ctx := context.Background()
	key := "rate:metrics:ip:" + ip

	if rdb != nil {
		n, err := rdb.Incr(ctx, key).Result()
		if err == nil {
			if n == 1 {
				_ = rdb.Expire(ctx, key, window).Err()
			}
			if n > int64(limit) {
				logging.WithComponent(logging.ComponentRateLimit).WithField(logging.FieldEvent, "metrics_rate_limit").WithField(logging.FieldIP, ip).Warn("metrics scrape rate limit exceeded (Redis)")
				errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
				c.Abort()
				return true
			}
			return false
		}
		logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "metrics_redis_incr_failed").Warn("metrics rate limit Redis incr failed; using in-memory fallback")
	}

	metricsRLMutex.Lock()
	defer metricsRLMutex.Unlock()
	now := time.Now()
	if reset, ok := metricsRLReset[ip]; ok && now.After(reset) {
		metricsRLCount[ip] = 0
		metricsRLReset[ip] = now.Add(window)
	}
	if _, ok := metricsRLCount[ip]; !ok {
		metricsRLCount[ip] = 0
		metricsRLReset[ip] = now.Add(window)
	}
	metricsRLCount[ip]++
	if metricsRLCount[ip] > limit {
		logging.WithComponent(logging.ComponentRateLimit).WithField(logging.FieldEvent, "metrics_rate_limit").WithField(logging.FieldIP, ip).Warn("metrics scrape rate limit exceeded (memory)")
		errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		c.Abort()
		return true
	}
	return false
}

// rateLimitMiddleware limits requests per IP and per authenticated user.
// It prefers a Redis-backed implementation for multi-instance resilience,
// and falls back to an in-memory limiter when Redis is unavailable.
//
// Exempt paths (orchestrator probes, Prometheus): see docs/RATE_LIMITS.md.
func rateLimitMiddleware(c *gin.Context) {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/debug/pprof") {
		c.Next()
		return
	}
	if path == "/metrics" {
		if enforceMetricsUnauthenticatedRateLimit(c) {
			return
		}
		c.Next()
		return
	}
	if path == "/healthz" || path == "/readyz" {
		c.Next()
		return
	}
	ip := c.ClientIP()
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)

	// Configurable limits via configuration (with sane defaults if config is nil)
	windowSeconds := 60
	perIPLimit := 10
	perUserLimit := 10
	if appConfig != nil {
		if appConfig.RateLimitWindowSeconds > 0 {
			windowSeconds = appConfig.RateLimitWindowSeconds
		}
		if appConfig.RateLimitPerIP > 0 {
			perIPLimit = appConfig.RateLimitPerIP
		}
		if appConfig.RateLimitPerUser > 0 {
			perUserLimit = appConfig.RateLimitPerUser
		}
	}
	window := time.Duration(windowSeconds) * time.Second

	// Prefer Redis-backed rate limiting when available
	if rdb != nil {
		ctx := context.Background()
		var exceeded bool

		// Per-IP limiting
		if perIPLimit > 0 {
			ipKey := fmt.Sprintf("rate:ip:%s", ip)
			ipCount, err := rdb.Incr(ctx, ipKey).Result()
			if err == nil {
				if ipCount == 1 {
					_ = rdb.Expire(ctx, ipKey, window).Err()
				}
				if ipCount > int64(perIPLimit) {
					exceeded = true
				}
			} else {
				logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_incr_failed").Warn("redis rate limit per IP failed; falling back to in-memory limiter")
			}
		}

		// Per-user limiting (only if authenticated)
		if !exceeded && username != "" && perUserLimit > 0 {
			userKey := fmt.Sprintf("rate:user:%s", username)
			userCount, err := rdb.Incr(ctx, userKey).Result()
			if err == nil {
				if userCount == 1 {
					_ = rdb.Expire(ctx, userKey, window).Err()
				}
				if userCount > int64(perUserLimit) {
					exceeded = true
				}
			} else {
				logging.WithComponent(logging.ComponentRateLimit).WithError(err).WithField(logging.FieldEvent, "redis_incr_failed").Warn("redis rate limit per user failed; falling back to in-memory limiter")
			}
		}

		if exceeded {
			log.WithFields(log.Fields{
				logging.FieldComponent: logging.ComponentRateLimit,
				logging.FieldEvent:     "api_rate_limit",
				logging.FieldIP:        ip,
				logging.FieldUsername:  username,
				"backend":              "redis",
			}).Warn("Rate limit exceeded (Redis)")
			errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
			c.Abort()
			return
		}

		// If Redis calls succeeded and limits not exceeded, proceed.
		if err := rdb.Ping(ctx).Err(); err == nil {
			c.Next()
			return
		}
		// If Redis is unhealthy, fall through to in-memory limiter.
	}

	// In-memory fallback (per IP only, 10 req/min default)
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()
	if reset, ok := rateLimitReset[ip]; ok && now.After(reset) {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(window)
	}
	if _, ok := rateLimitCount[ip]; !ok {
		rateLimitCount[ip] = 0
		rateLimitReset[ip] = now.Add(window)
	}
	rateLimitCount[ip]++
	if rateLimitCount[ip] > perIPLimit {
		log.WithFields(log.Fields{
			logging.FieldComponent: logging.ComponentRateLimit,
			logging.FieldEvent:     "api_rate_limit",
			logging.FieldIP:        ip,
			"backend":              "memory",
		}).Warn("Rate limit exceeded (in-memory)")
		errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
		c.Abort()
		return
	}
	c.Next()
}

/* Duplicate authentication handlers removed. The remaining/primary handlers are defined elsewhere in the file. */

// InvalidCachedJSONError is returned when cached []byte exists but cannot be unmarshaled.
type InvalidCachedJSONError struct {
	TxID string
	Err  error
}

func (e *InvalidCachedJSONError) Error() string {
	return fmt.Sprintf("invalid cached JSON for transaction %s: %v", e.TxID, e.Err)
}

func (e *InvalidCachedJSONError) Unwrap() error { return e.Err }

// IsInvalidCachedJSON returns true if err is (or wraps) InvalidCachedJSONError
func IsInvalidCachedJSON(err error) bool {
	var target *InvalidCachedJSONError
	return errors.As(err, &target)
}

// GET /api/news/:symbol and GET /api/v1/news/:symbol
// Public endpoint; returns deduplicated articles for the given symbol/keyword.
func newsBySymbolHandler(c *gin.Context) {
	if newsService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "news_unavailable", "News service is not configured")
		return
	}
	symbol := strings.TrimSpace(c.Param("symbol"))
	if symbol == "" || len(symbol) > 50 {
		errorResponse(c, http.StatusBadRequest, "invalid_symbol", "Symbol must be 1-50 characters")
		return
	}
	symbol = sanitizeText(symbol, 50)

	limit := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSizeNews).PageSize

	// Build query: caller symbol plus default search (if configured).
	query := buildNewsQueryForSymbol(symbol, appConfig)
	extras := newsExtrasFromConfig(appConfig)
	key := news.CacheKey(newsService.ProviderName(), "news:symbol", query, extras)

	articles, cached, stale, err := newsService.Get(c.Request.Context(), key, query, limit)
	if err != nil {
		// Provider error: we already attempted stale fallback inside service.
		status := http.StatusBadGateway
		if errors.Is(err, news.ErrRateLimited) {
			status = http.StatusTooManyRequests
		}
		errorResponse(c, status, "news_fetch_failed", "Failed to fetch news")
		return
	}
	metrics.RecordNews(cached, stale)

	user := getOptionalAuthenticatedUser(c)
	favoritesOnly := strings.ToLower(strings.TrimSpace(c.Query("favorites_only"))) == "true"
	articles = applyUserNewsPrefs(articles, user, favoritesOnly)

	resp := news.ListResponse{
		Data: articles,
		Meta: news.Meta{
			Provider: newsService.ProviderName(),
			Cached:   cached,
			Stale:    stale,
			Query:    query,
		},
	}
	if _, err := writeJSONConditional(c, resp, "public, max-age=120", "news_symbol", nil); err != nil {
		errorResponse(c, http.StatusInternalServerError, "marshal_failed", "Failed to marshal response")
		return
	}
}

// GET /api/news/portfolio/:id and GET /api/v1/news/portfolio/:id
// Authenticated endpoint; returns deduplicated articles relevant to the portfolio's assets.
func newsByPortfolioHandler(c *gin.Context) {
	if newsService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "news_unavailable", "News service is not configured")
		return
	}
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	portfolioID := strings.TrimSpace(c.Param("id"))
	if portfolioID == "" || len(portfolioID) > 64 {
		errorResponse(c, http.StatusBadRequest, "invalid_portfolio_id", "Portfolio id must be 1-64 characters")
		return
	}

	if appRepos == nil || appRepos.Portfolio == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Redis unavailable")
		return
	}
	data, err := appRepos.Portfolio.Get(ctx, username, portfolioID)
	if err != nil {
		if err == redis.Nil {
			errorResponse(c, http.StatusNotFound, "portfolio_not_found", "Portfolio not found")
		} else {
			errorResponse(c, http.StatusInternalServerError, "portfolio_fetch_failed", "Failed to load portfolio")
		}
		return
	}
	var p Portfolio
	if err := json.Unmarshal(data, &p); err != nil {
		errorResponse(c, http.StatusInternalServerError, "portfolio_decode_failed", "Failed to load portfolio")
		return
	}

	limit := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSizeNews).PageSize

	query := buildNewsQueryForPortfolio(&p, appConfig)
	if query == "" {
		errorResponse(c, http.StatusBadRequest, "portfolio_empty", "Portfolio has no searchable assets")
		return
	}
	extras := newsExtrasFromConfig(appConfig)
	cacheKey := news.CacheKey(newsService.ProviderName(), "news:portfolio:"+portfolioID, query, extras)

	articles, cached, stale, err := newsService.Get(c.Request.Context(), cacheKey, query, limit)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, news.ErrRateLimited) {
			status = http.StatusTooManyRequests
		}
		errorResponse(c, status, "news_fetch_failed", "Failed to fetch news")
		return
	}
	metrics.RecordNews(cached, stale)

	u := getOptionalAuthenticatedUser(c)
	favoritesOnly := strings.ToLower(strings.TrimSpace(c.Query("favorites_only"))) == "true"
	articles = applyUserNewsPrefs(articles, u, favoritesOnly)

	resp := news.ListResponse{
		Data: articles,
		Meta: news.Meta{
			Provider: newsService.ProviderName(),
			Cached:   cached,
			Stale:    stale,
			Query:    query,
		},
	}
	if _, err := writeJSONConditional(c, resp, "private, max-age=60", "news_portfolio", nil); err != nil {
		errorResponse(c, http.StatusInternalServerError, "marshal_failed", "Failed to marshal response")
		return
	}
}

func buildNewsQueryForSymbol(symbol string, cfg *config.Config) string {
	s := strings.TrimSpace(symbol)
	if s == "" {
		return ""
	}
	// TheNewsAPI query language uses | for OR and quotes for phrases.
	// We keep it simple: symbol or quoted symbol plus optional default search.
	quoted := `"` + strings.ReplaceAll(s, `"`, "") + `"`
	q := "(" + s + " | " + quoted + ")"
	if cfg != nil {
		if base := strings.TrimSpace(cfg.TheNewsAPIDefaultSearch); base != "" {
			q = q + " + (" + base + ")"
		}
	}
	return q
}

func buildNewsQueryForPortfolio(p *Portfolio, cfg *config.Config) string {
	if p == nil || len(p.Items) == 0 {
		return ""
	}
	terms := make([]string, 0, len(p.Items))
	seen := make(map[string]bool, len(p.Items))
	for _, it := range p.Items {
		// Prefer symbol; fall back to label.
		sym := strings.TrimSpace(it.Symbol)
		if sym == "" {
			sym = strings.TrimSpace(it.Label)
		}
		sym = sanitizeText(sym, 50)
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		k := strings.ToLower(sym)
		if seen[k] {
			continue
		}
		seen[k] = true
		terms = append(terms, sym)
		if len(terms) >= 10 {
			break
		}
	}
	if len(terms) == 0 {
		return ""
	}
	// Build OR query: (a | "a" | b | "b" ...)
	var parts []string
	for _, t := range terms {
		clean := strings.ReplaceAll(t, `"`, "")
		parts = append(parts, clean, `"`+clean+`"`)
	}
	q := "(" + strings.Join(parts, " | ") + ")"
	if cfg != nil {
		if base := strings.TrimSpace(cfg.TheNewsAPIDefaultSearch); base != "" {
			q = q + " + (" + base + ")"
		}
	}
	return q
}

func newsExtrasFromConfig(cfg *config.Config) map[string]string {
	if cfg == nil {
		return nil
	}
	out := map[string]string{}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultLanguage); v != "" {
		out["language"] = v
	}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultLocale); v != "" {
		out["locale"] = v
	}
	if v := strings.TrimSpace(cfg.TheNewsAPIDefaultCategories); v != "" {
		out["categories"] = v
	}
	return out
}

func normalizeNewsSourceList(in []string, maxItems int) ([]string, error) {
	if maxItems <= 0 {
		maxItems = 50
	}
	if len(in) > maxItems {
		return nil, fmt.Errorf("too many sources (max %d)", maxItems)
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		if len(s) > 100 {
			return nil, fmt.Errorf("source too long (max 100 chars)")
		}
		// Basic allowlist for host-like identifiers (domain or subdomain).
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
				continue
			}
			return nil, fmt.Errorf("invalid source %q (use domains like reuters.com)", raw)
		}
		if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.Contains(s, "..") {
			return nil, fmt.Errorf("invalid source %q (use domains like reuters.com)", raw)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	// Keep stable order as provided by user.
	return out, nil
}

func getOptionalAuthenticatedUser(c *gin.Context) *User {
	if c == nil {
		return nil
	}
	// If upstream middleware already set username, use it.
	if v, ok := c.Get("username"); ok {
		if username, ok2 := v.(string); ok2 && username != "" {
			if u, ok3 := getUser(username); ok3 {
				return &u
			}
		}
		return nil
	}
	// Best-effort session cookie lookup; do not abort if missing/invalid.
	sessionID, err := c.Cookie("session_id")
	if err != nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	username, valid := validateSession(sessionID)
	if !valid || strings.TrimSpace(username) == "" {
		return nil
	}
	u, ok := getUser(username)
	if !ok {
		return nil
	}
	return &u
}

func applyUserNewsPrefs(articles []news.Article, user *User, favoritesOnly bool) []news.Article {
	if user == nil || len(articles) == 0 {
		return articles
	}
	blocked := make(map[string]bool, len(user.NewsSourcesBlocked))
	for _, s := range user.NewsSourcesBlocked {
		blocked[strings.ToLower(strings.TrimSpace(s))] = true
	}
	favs := make(map[string]bool, len(user.NewsSourcesFavorite))
	for _, s := range user.NewsSourcesFavorite {
		favs[strings.ToLower(strings.TrimSpace(s))] = true
	}

	filtered := make([]news.Article, 0, len(articles))
	for _, a := range articles {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src != "" && blocked[src] {
			continue
		}
		if favoritesOnly {
			if src == "" || !favs[src] {
				continue
			}
		}
		filtered = append(filtered, a)
	}
	if favoritesOnly || len(favs) == 0 || len(filtered) == 0 {
		return filtered
	}
	// Stable partition: favorites first, then the rest; keep relative order within groups.
	out := make([]news.Article, 0, len(filtered))
	for _, a := range filtered {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src != "" && favs[src] {
			out = append(out, a)
		}
	}
	for _, a := range filtered {
		src := strings.ToLower(strings.TrimSpace(a.Source))
		if src == "" || !favs[src] {
			out = append(out, a)
		}
	}
	return out
}

// -----------------------------
// Price alerts (Redis-backed)
// -----------------------------

const (
	priceAlertKeyPrefix   = "alert:"
	notificationKeyPrefix = "notification:"
)

type createPriceAlertRequest struct {
	Symbol         string  `json:"symbol"`
	Currency       string  `json:"currency"`
	Threshold      float64 `json:"threshold"`
	Direction      string  `json:"direction"`       // above|below
	DeliveryMethod string  `json:"delivery_method"` // in_app|email
	IsActive       *bool   `json:"is_active,omitempty"`
}

type updatePriceAlertRequest struct {
	Symbol         *string  `json:"symbol,omitempty"`
	Currency       *string  `json:"currency,omitempty"`
	Threshold      *float64 `json:"threshold,omitempty"`
	Direction      *string  `json:"direction,omitempty"`
	DeliveryMethod *string  `json:"delivery_method,omitempty"`
	IsActive       *bool    `json:"is_active,omitempty"`
}

func listPriceAlertsHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Alerts require Redis")
		return
	}

	alerts := make([]PriceAlert, 0, 32)
	var cursor uint64
	pattern := priceAlertKeyPrefix + username + ":*"
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, "alerts_fetch_failed", "Failed to fetch alerts")
			return
		}
		cursor = next
		for _, key := range keys {
			raw, err := rdb.Get(ctx, key).Result()
			if err != nil || raw == "" {
				continue
			}
			var a PriceAlert
			if err := json.Unmarshal([]byte(raw), &a); err == nil {
				alerts = append(alerts, a)
			}
		}
		if cursor == 0 {
			break
		}
	}

	// Newest first
	sort.Slice(alerts, func(i, j int) bool {
		return alerts[i].Created.After(alerts[j].Created)
	})

	pagination := apiutil.ParsePagination(c, apiutil.DefaultPageSize, apiutil.MaxPageSize)
	total := len(alerts)
	start := pagination.Offset
	if start > total {
		start = total
	}
	end := start + pagination.PageSize
	if end > total {
		end = total
	}
	pageSlice := alerts[start:end]

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

func createPriceAlertHandler(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if strings.TrimSpace(username) == "" {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", "Login required")
		return
	}
	if rdb == nil {
		errorResponse(c, http.StatusServiceUnavailable, "storage_unavailable", "Alerts require Redis")
		return
	}

	var req createPriceAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	alert, err := buildPriceAlertFromCreate(username, req)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "invalid_alert", err.Error())
		return
	}

	key := priceAlertKeyPrefix + username + ":" + alert.ID
	data, _ := json.Marshal(alert)
	if err := rdb.Set(ctx, key, data, 0).Err(); err != nil {
		errorResponse(c, http.StatusInternalServerError, "alert_save_failed", "Failed to save alert")
		return
	}
	c.JSON(http.StatusCreated, alert)
}
