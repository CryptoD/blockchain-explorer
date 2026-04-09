package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/gin-gonic/gin"
)

func TestRateLimit_HealthzExempt(t *testing.T) {
	old := appConfig
	defer func() { appConfig = old }()
	rdb.FlushDB(ctx)
	appConfig = &config.Config{
		RateLimitPerIP:         2,
		RateLimitWindowSeconds: 60,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware)
	r.GET("/health", livenessHandler)
	r.GET("/healthz", livenessHandler)
	r.GET("/ready", readinessHandler)
	r.GET("/readyz", readinessHandler)

	paths := []string{"/health", "/healthz", "/ready", "/readyz"}
	for i := 0; i < 25; i++ {
		p := paths[i%len(paths)]
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d path %s: %d %s", i, p, w.Code, w.Body.String())
		}
	}
}

func TestRateLimit_RegularRoutesStillEnforced(t *testing.T) {
	old := appConfig
	defer func() { appConfig = old }()
	rdb.FlushDB(ctx)
	appConfig = &config.Config{
		RateLimitPerIP:         2,
		RateLimitWindowSeconds: 60,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware)
	r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
}

func TestRateLimit_MetricsUnauthenticatedSeparateBudget(t *testing.T) {
	old := appConfig
	defer func() { appConfig = old }()
	rdb.FlushDB(ctx)
	appConfig = &config.Config{
		RateLimitPerIP:         2,
		RateLimitWindowSeconds: 60,
		MetricsEnabled:         true,
		MetricsToken:           "",
		MetricsRateLimitPerIP:  3,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware)
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("metrics want 429 on 4th, got %d", w.Code)
	}
}

func TestRateLimit_MetricsWithTokenSkipsUnauthenticatedMetricsLimit(t *testing.T) {
	old := appConfig
	defer func() { appConfig = old }()
	rdb.FlushDB(ctx)
	appConfig = &config.Config{
		RateLimitPerIP:         2,
		RateLimitWindowSeconds: 60,
		MetricsEnabled:         true,
		MetricsToken:           "secret-token",
		MetricsRateLimitPerIP:  3,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(rateLimitMiddleware)
	r.GET("/metrics", metrics.TokenAuthMiddleware("secret-token"), gin.WrapH(metrics.Handler()))

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set("Authorization", "Bearer secret-token")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: %d", i, w.Code)
		}
	}
}
