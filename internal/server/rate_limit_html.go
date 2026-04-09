package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// wantsHTMLRateLimitPage is true for browser-style navigations that should not receive JSON 429 bodies.
// API and static asset paths keep JSON so fetch/XHR clients stay consistent.
func wantsHTMLRateLimitPage(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}
	p := c.Request.URL.Path
	if strings.HasPrefix(p, "/api/") {
		return false
	}
	if strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/dist/") || strings.HasPrefix(p, "/images/") {
		return false
	}
	switch p {
	case "/", "/dashboard", "/profile", "/symbols", "/admin", "/bitcoin", "/bitcoin.html":
		return true
	}
	if strings.HasSuffix(p, ".html") {
		return true
	}
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}

const htmlRateLimitBody = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Too many requests</title>
<style>
body{font-family:system-ui,sans-serif;margin:0;padding:2rem;background:#f8fafc;color:#0f172a;line-height:1.5}
.box{max-width:36rem;margin:10vh auto;padding:1.5rem 1.75rem;border:1px solid #e2e8f0;border-radius:8px;background:#fff;box-shadow:0 1px 3px rgba(0,0,0,.08)}
h1{font-size:1.25rem;margin:0 0 .5rem}
p{margin:0;font-size:.95rem;color:#334155}
</style>
</head>
<body>
<div class="box">
<h1>Too many requests</h1>
<p>You’ve sent too many requests from this browser in a short time. Please wait a minute and try again.</p>
<p>This page is shown instead of raw JSON so you’re not stuck looking at an API error while browsing the site.</p>
</div>
</body>
</html>
`

// writeHTMLRateLimitResponse sends a human-readable 429 for HTML navigations (task 62).
func writeHTMLRateLimitResponse(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Retry-After", "60")
	c.Status(http.StatusTooManyRequests)
	_, _ = c.Writer.WriteString(htmlRateLimitBody)
}

// rateLimitErrorResponse writes JSON for APIs, HTML for browser page loads.
func rateLimitErrorResponse(c *gin.Context) {
	if wantsHTMLRateLimitPage(c) {
		writeHTMLRateLimitResponse(c)
		return
	}
	errorResponse(c, http.StatusTooManyRequests, "rate_limited", "Too many requests")
}
