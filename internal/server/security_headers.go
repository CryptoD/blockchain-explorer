package server

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

// securityHeadersMiddleware sets OWASP-aligned HTTP response headers for HTML and API responses.
// HSTS is emitted only when the request is HTTPS (TLS or X-Forwarded-Proto: https) and
// cfg.HSTSMaxAgeSeconds > 0 — set HSTS_MAX_AGE_SECONDS in production behind TLS (see docs/SECURITY_HEADERS.md).
func securityHeadersMiddleware(cfg *config.Config) gin.HandlerFunc {
	csp := buildContentSecurityPolicy(cfg)
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

// buildContentSecurityPolicy matches static pages under /static/js and CDNs (Chart.js, qrcode).
// No script 'unsafe-inline': page logic lives in external files (roadmap task 39). Styles may still use
// 'unsafe-inline' where Tailwind or dynamic HTML requires it.
// When CDN_BASE_URL is set (same host used at build time for stamped HTML), allow that origin for scripts, styles, and images.
func buildContentSecurityPolicy(cfg *config.Config) string {
	scriptSrc := "'self' https://cdn.jsdelivr.net"
	styleSrc := "'self'"
	imgSrc := "'self' data: https:"
	if cfg != nil {
		if o := cdnOriginForCSP(cfg.CDNBaseURL); o != "" {
			scriptSrc += " " + o
			styleSrc += " " + o
			imgSrc += " " + o
		}
	}
	return "default-src 'self'; " +
		"script-src " + scriptSrc + "; " +
		"style-src " + styleSrc + " 'unsafe-inline'; " +
		"img-src " + imgSrc + "; " +
		"font-src 'self'; " +
		"connect-src 'self'; " +
		"frame-ancestors 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"object-src 'none'"
}

func cdnOriginForCSP(cdnBaseURL string) string {
	u, err := url.Parse(strings.TrimSpace(cdnBaseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
