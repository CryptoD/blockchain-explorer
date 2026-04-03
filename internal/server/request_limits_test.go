package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

func TestRequestBodyLimits_JSONPayloadTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{MaxRequestBodyBytes: 80}
	r := gin.New()
	r.Use(requestBodyLimitsMiddleware(cfg))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := strings.Repeat("a", 200)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d body %s", w.Code, w.Body.String())
	}
}

func TestRequestBodyLimits_JSONTooDeep(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{MaxRequestBodyBytes: 4096, MaxJSONDepth: 3}
	r := gin.New()
	r.Use(requestBodyLimitsMiddleware(cfg))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := `{"a":{"b":{"c":{"d":1}}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body %s", w.Code, w.Body.String())
	}
}

func TestRequestBodyLimits_JSONOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{MaxRequestBodyBytes: 4096, MaxJSONDepth: 8}
	r := gin.New()
	r.Use(requestBodyLimitsMiddleware(cfg))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := `{"a":{"b":{"c":{"d":1}}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
	}
}
