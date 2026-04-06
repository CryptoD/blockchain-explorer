package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

func TestStaticAssetCacheMiddleware_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(staticAssetCacheMiddleware(&config.Config{StaticAssetCacheMaxAgeSeconds: 0}))
	r.GET("/static/js/x.js", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/static/js/x.js", nil))
	if w.Header().Get("Cache-Control") != "" {
		t.Fatalf("expected no Cache-Control when disabled, got %q", w.Header().Get("Cache-Control"))
	}
}

func TestStaticAssetCacheMiddleware_Enabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(staticAssetCacheMiddleware(&config.Config{StaticAssetCacheMaxAgeSeconds: 31536000}))
	r.GET("/dist/styles.css", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dist/styles.css", nil))
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
