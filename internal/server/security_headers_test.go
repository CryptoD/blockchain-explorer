package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersMiddleware_Baseline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{HSTSMaxAgeSeconds: 0}
	r := gin.New()
	r.Use(securityHeadersMiddleware(cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	h := w.Header()
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := h.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q", got)
	}
	if got := h.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
	if got := h.Get("Content-Security-Policy"); got == "" {
		t.Fatal("missing Content-Security-Policy")
	}
	if h.Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS should not be set without HTTPS or when max-age is 0")
	}
}

func TestSecurityHeadersMiddleware_HSTS_https(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{HSTSMaxAgeSeconds: 31536000, HSTSIncludeSubdomains: true}
	r := gin.New()
	r.Use(securityHeadersMiddleware(cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	r.ServeHTTP(w, req)

	sts := w.Header().Get("Strict-Transport-Security")
	if sts != "max-age=31536000; includeSubDomains" {
		t.Fatalf("Strict-Transport-Security = %q", sts)
	}
}

func TestBuildContentSecurityPolicy_ScriptsExternalAndCDN(t *testing.T) {
	csp := buildContentSecurityPolicy()
	for _, needle := range []string{
		"default-src 'self'",
		"script-src 'self' https://cdn.jsdelivr.net",
		"style-src 'self' 'unsafe-inline'",
		"cdn.jsdelivr.net",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, needle) {
			t.Fatalf("CSP missing %q: %s", needle, csp)
		}
	}
	if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("script-src should not allow inline scripts: %s", csp)
	}
}
