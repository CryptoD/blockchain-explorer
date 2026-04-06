package server

import (
	"fmt"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

// staticAssetCacheMiddleware sets aggressive caching for fingerprinted static paths when
// STATIC_ASSET_CACHE_MAX_AGE_SECONDS > 0 (see docs/CDN_STATIC_ASSETS.md, roadmap 49).
func staticAssetCacheMiddleware(cfg *config.Config) gin.HandlerFunc {
	if cfg == nil || cfg.StaticAssetCacheMaxAgeSeconds <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	maxAge := cfg.StaticAssetCacheMaxAgeSeconds
	val := fmt.Sprintf("public, max-age=%d, immutable", maxAge)
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/dist/") || strings.HasPrefix(p, "/images/") {
			c.Writer.Header().Set("Cache-Control", val)
		}
		c.Next()
	}
}
