package server

import (
	"fmt"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

// securityHeadersMiddleware sets OWASP-aligned HTTP response headers for HTML and API responses.
// HSTS is emitted only when the request is HTTPS (TLS or X-Forwarded-Proto: https) and
// cfg.HSTSMaxAgeSeconds > 0 — set HSTS_MAX_AGE_SECONDS in production behind TLS (see docs/SECURITY_HEADERS.md).
func securityHeadersMiddleware(cfg *config.Config) gin.HandlerFunc {
	csp := buildContentSecurityPolicy()
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Narrow sensitive browser features; expand only if the product needs them.
		h.Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), magnetometer=(), microphone=(), payment=(), usb=()")
		h.Set("Content-Security-Policy", csp)

		if cfg != nil && cfg.HSTSMaxAgeSeconds > 0 && isHTTPSRequest(c) {
			v := fmt.Sprintf("max-age=%d", cfg.HSTSMaxAgeSeconds)
			if cfg.HSTSIncludeSubdomains {
				v += "; includeSubDomains"
			}
			h.Set("Strict-Transport-Security", v)
		}
		c.Next()
	}
}

func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https")
}

// buildContentSecurityPolicy matches current static pages: inline scripts, /script.js, cdn.jsdelivr.net (Chart.js, qrcode).
// 'unsafe-inline' for script/style is a known gap; roadmap task 39 tracks tightening (nonces/hashes).
func buildContentSecurityPolicy() string {
	const (
		// semicolons separate directives; keep single line for one header value
		policy = "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data: https:; " +
			"font-src 'self'; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'; " +
			"base-uri 'self'; " +
			"form-action 'self'; " +
			"object-src 'none'"
	)
	return policy
}
