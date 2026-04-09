package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

func TestWantsHTMLRateLimitPage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path   string
		accept string
		want   bool
	}{
		{"/api/v1/search", "text/html", false},
		{"/", "text/html", true},
		{"/dashboard", "", true},
		{"/static/js/x.js", "text/html", false},
		{"/dist/styles.css", "text/html", false},
		{"/foo", "application/json", false},
		{"/unknown", "text/html", true},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.accept != "" {
			req.Header.Set("Accept", tc.accept)
		}
		c.Request = req
		got := wantsHTMLRateLimitPage(c)
		if got != tc.want {
			t.Fatalf("path %q Accept %q: got %v want %v", tc.path, tc.accept, got, tc.want)
		}
	}
}

func TestRateLimit_HTMLPageGetsHTML429(t *testing.T) {
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
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "<!DOCTYPE html>") {
		t.Fatalf("expected HTML body, got %q", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type: %q", ct)
	}
}

func TestRateLimit_APIStillJSON429(t *testing.T) {
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
	r.GET("/api/v1/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Accept", "application/json")
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("iter %d: %d", i, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected JSON, got %q", ct)
	}
}
