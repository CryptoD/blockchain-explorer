package server

import (
	"net/http"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/gin-gonic/gin"
)

// Route registration is split by bounded context so Run() does not contain a single mega route list (ROADMAP task 5).

func registerStaticRoutes(r *gin.Engine) {
	r.Static("/images", "./images")
	r.Static("/dist", "./dist")
	r.Static("/static", "./static")
	r.StaticFile("/bitcoin.html", "bitcoin.html")
	r.StaticFile("/", "index.html")
	r.StaticFile("/admin", "admin.html")
	r.StaticFile("/dashboard", "dashboard.html")
	r.StaticFile("/profile", "profile.html")
	r.StaticFile("/symbols", "symbols.html")

	r.GET("/bitcoin", func(c *gin.Context) {
		query := c.Query("q")
		c.Redirect(http.StatusFound, "/bitcoin.html?q="+query)
	})
}

func registerHealthAndMetricsRoutes(r *gin.Engine, cfg *config.Config) {
	r.GET("/healthz", healthHandler)
	r.GET("/readyz", readinessHandler)

	if cfg.MetricsEnabled {
		if cfg.MetricsToken != "" {
			r.GET("/metrics", metrics.TokenAuthMiddleware(cfg.MetricsToken), gin.WrapH(metrics.Handler()))
		} else {
			r.GET("/metrics", gin.WrapH(metrics.Handler()))
		}
	}
}

// registerAPIV1Routes mounts all /api/v1/* handlers.
func registerAPIV1Routes(r *gin.Engine) {
	apiV1 := r.Group("/api/v1")
	registerExplorerRoutesV1(apiV1)
	registerFeedbackRoutesV1(apiV1)
	registerNewsRoutesV1(apiV1)
	registerAuthRoutesV1(apiV1)
	registerUserRoutesV1(apiV1)
	registerAdminRoutesV1(apiV1)
}

// Explorer domain: search, exports, chain data, rates, autocomplete.
func registerExplorerRoutesV1(apiV1 *gin.RouterGroup) {
	apiV1.GET("/search", searchHandler)
	apiV1.GET("/search/export", exportSearchHandler)
	apiV1.GET("/search/advanced", advancedSearchHandler)
	apiV1.GET("/search/advanced/export", exportAdvancedSearchHandler)
	apiV1.GET("/search/categories", getSymbolCategoriesHandler)
	apiV1.GET("/blocks/export/csv", exportBlocksCSVHandler)
	apiV1.GET("/transactions/export/csv", exportTransactionsCSVHandler)
	apiV1.GET("/autocomplete", autocompleteHandler)
	apiV1.GET("/metrics", metricsHandler)
	apiV1.GET("/network-status", networkStatusHandler)
	apiV1.GET("/rates", ratesHandler)
	apiV1.GET("/price-history", priceHistoryHandler)
}

func registerFeedbackRoutesV1(apiV1 *gin.RouterGroup) {
	apiV1.POST("/feedback", feedbackHandler)
}

// News domain: public symbol news + authenticated portfolio-scoped news.
func registerNewsRoutesV1(apiV1 *gin.RouterGroup) {
	apiV1.GET("/news/:symbol", newsBySymbolHandler)

	newsAuth := apiV1.Group("/news")
	newsAuth.Use(authMiddleware)
	{
		newsAuth.GET("/portfolio/:id", newsByPortfolioHandler)
	}
}

// Auth domain: login, logout, register (no session middleware on these paths).
func registerAuthRoutesV1(apiV1 *gin.RouterGroup) {
	apiV1.POST("/login", loginHandler)
	apiV1.POST("/logout", logoutHandler)
	apiV1.POST("/register", registerHandler)
}

// User domain: profile, notifications, alerts, portfolios, watchlists (session + CSRF via global middleware).
func registerUserRoutesV1(apiV1 *gin.RouterGroup) {
	user := apiV1.Group("/user")
	user.Use(authMiddleware)
	registerUserProfileRoutes(user)
	registerUserNotificationRoutes(user)
	registerUserPriceAlertRoutes(user)
	registerUserPortfolioRoutes(user)
	registerUserWatchlistRoutes(user)
}

func registerUserProfileRoutes(user *gin.RouterGroup) {
	user.GET("/profile", userProfileHandler)
	user.PATCH("/profile", updateProfileHandler)
	user.PATCH("/password", changePasswordHandler)
}

func registerUserNotificationRoutes(user *gin.RouterGroup) {
	user.GET("/notifications", listNotificationsHandler)
	user.PUT("/notifications/:id", updateNotificationHandler)
	user.DELETE("/notifications/:id", dismissNotificationHandler)
}

func registerUserPriceAlertRoutes(user *gin.RouterGroup) {
	user.GET("/alerts", listPriceAlertsHandler)
	user.POST("/alerts", createPriceAlertHandler)
	user.PUT("/alerts/:id", updatePriceAlertHandler)
	user.DELETE("/alerts/:id", deletePriceAlertHandler)
}

func registerUserPortfolioRoutes(user *gin.RouterGroup) {
	user.GET("/portfolios", listPortfoliosHandler)
	user.GET("/portfolios/export", exportPortfoliosHandler)
	user.GET("/portfolios/:id/export/csv", exportPortfolioCSVHandler)
	user.GET("/portfolios/:id/export/pdf", exportPortfolioPDFHandler)
	user.POST("/portfolios", createPortfolioHandler)
	user.PUT("/portfolios/:id", updatePortfolioHandler)
	user.DELETE("/portfolios/:id", deletePortfolioHandler)
}

func registerUserWatchlistRoutes(user *gin.RouterGroup) {
	user.GET("/watchlists", listWatchlistsHandler)
	user.GET("/watchlists/:id", getWatchlistHandler)
	user.POST("/watchlists", createWatchlistHandler)
	user.PUT("/watchlists/:id", updateWatchlistHandler)
	user.DELETE("/watchlists/:id", deleteWatchlistHandler)
	user.POST("/watchlists/:id/entries", addWatchlistEntryHandler)
	user.PUT("/watchlists/:id/entries/:index", updateWatchlistEntryHandler)
	user.DELETE("/watchlists/:id/entries/:index", deleteWatchlistEntryHandler)
}

// Admin domain: operational status and cache (RBAC: admin role).
func registerAdminRoutesV1(apiV1 *gin.RouterGroup) {
	admin := apiV1.Group("/admin")
	admin.Use(authMiddleware)
	admin.Use(requireRoleMiddleware("admin"))
	{
		admin.GET("/status", adminStatusHandler)
		admin.GET("/cache", adminCacheHandler)
	}
}

// registerLegacyAPIRoutes keeps non-versioned /api/* paths for backward compatibility.
func registerLegacyAPIRoutes(r *gin.Engine) {
	r.GET("/api/search", searchHandler)
	r.GET("/api/search/export", exportSearchHandler)
	r.GET("/api/search/advanced", advancedSearchHandler)
	r.GET("/api/search/advanced/export", exportAdvancedSearchHandler)
	r.GET("/api/search/categories", getSymbolCategoriesHandler)
	r.GET("/api/blocks/export/csv", exportBlocksCSVHandler)
	r.GET("/api/transactions/export/csv", exportTransactionsCSVHandler)

	r.GET("/api/autocomplete", autocompleteHandler)
	r.GET("/api/metrics", metricsHandler)
	r.GET("/api/network-status", networkStatusHandler)
	r.GET("/api/rates", ratesHandler)
	r.GET("/api/price-history", priceHistoryHandler)

	r.GET("/api/news/:symbol", newsBySymbolHandler)

	r.POST("/api/feedback", feedbackHandler)

	r.POST("/api/login", loginHandler)
	r.POST("/api/logout", logoutHandler)
	r.POST("/api/register", registerHandler)

	user := r.Group("/api/user")
	user.Use(authMiddleware)
	registerUserProfileRoutes(user)
	registerUserNotificationRoutes(user)
	registerUserPriceAlertRoutes(user)
	registerUserPortfolioRoutes(user)
	registerUserWatchlistRoutes(user)

	newsLegacy := r.Group("/api/news")
	newsLegacy.Use(authMiddleware)
	{
		newsLegacy.GET("/portfolio/:id", newsByPortfolioHandler)
	}

	admin := r.Group("/api/admin")
	admin.Use(authMiddleware)
	admin.Use(requireRoleMiddleware("admin"))
	{
		admin.GET("/status", adminStatusHandler)
		admin.GET("/cache", adminCacheHandler)
	}
}
