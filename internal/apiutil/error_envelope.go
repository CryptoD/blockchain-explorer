package apiutil

import (
	"time"

	"github.com/gin-gonic/gin"
)

// ErrorEnvelopeJSON returns the standard JSON error object for HTTP API responses:
// code, message, correlation_id (may be empty if middleware did not set Gin context key "correlation_id"),
// and timestamp (RFC3339Nano in UTC).
func ErrorEnvelopeJSON(c *gin.Context, code, message string) gin.H {
	cid := ""
	if c != nil {
		if v, ok := c.Get("correlation_id"); ok {
			if s, ok := v.(string); ok && s != "" {
				cid = s
			}
		}
	}
	return gin.H{
		"code":            code,
		"message":         message,
		"correlation_id":  cid,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
	}
}
