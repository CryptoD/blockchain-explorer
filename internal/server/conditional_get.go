package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/metrics"
	"github.com/gin-gonic/gin"
)

// ifNoneMatchEquals reports whether If-None-Match matches a strong ETag (exact byte identity).
// Supports comma-separated lists. Does not treat "*" as a match (avoids spurious 304 on first GET).
func ifNoneMatchEquals(header string, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" || header == "*" {
		return false
	}
	for _, p := range strings.Split(header, ",") {
		if strings.TrimSpace(p) == etag {
			return true
		}
	}
	return false
}

func etagFromJSONBody(body []byte) string {
	return fmt.Sprintf("\"%x\"", sha256.Sum256(body))
}

// writeJSONConditional marshals payload, sets ETag (SHA-256 of JSON bytes) and optional Cache-Control.
// If If-None-Match matches, it sends 304 and returns notModified=true.
// metricsName is a low-cardinality suffix for explorer_cache_events_total (e.g. "search"); empty skips ETag metrics.
func writeJSONConditional(c *gin.Context, payload interface{}, cacheControl string, metricsName string, onNotModified func()) (notModified bool, err error) {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	etag := etagFromJSONBody(jsonBytes)
	c.Header("ETag", etag)
	if cc := strings.TrimSpace(cacheControl); cc != "" {
		c.Header("Cache-Control", cc)
	}
	if ifNoneMatchEquals(c.GetHeader("If-None-Match"), etag) {
		if onNotModified != nil {
			onNotModified()
		}
		if metricsName != "" {
			metrics.RecordHTTPETag304(metricsName, true)
		}
		c.Status(http.StatusNotModified)
		return true, nil
	}
	if metricsName != "" {
		metrics.RecordHTTPETag304(metricsName, false)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", jsonBytes)
	return false, nil
}
