package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/apiutil"
	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/gin-gonic/gin"
)

// requestBodyLimitsMiddleware enforces maximum request body size and optional JSON nesting depth
// for POST, PUT, and PATCH. See docs/INPUT_LIMITS.md.
func requestBodyLimitsMiddleware(cfg *config.Config) gin.HandlerFunc {
	maxBytes := int64(1024 * 1024) // 1 MiB when cfg is nil
	maxDepth := 64
	if cfg != nil {
		if cfg.MaxRequestBodyBytes > 0 {
			maxBytes = cfg.MaxRequestBodyBytes
		} else {
			maxBytes = 0 // unlimited (MAX_REQUEST_BODY_BYTES=0)
		}
		maxDepth = cfg.MaxJSONDepth
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}
		if maxBytes <= 0 {
			c.Next()
			return
		}
		if cl := c.Request.ContentLength; cl > maxBytes {
			errorResponse(c, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body too large")
			c.Abort()
			return
		}
		ct := strings.ToLower(c.ContentType())
		isJSON := strings.Contains(ct, "application/json")

		body := c.Request.Body
		defer body.Close()

		if isJSON {
			data, err := io.ReadAll(http.MaxBytesReader(c.Writer, body, maxBytes))
			if err != nil {
				if isRequestBodyTooLarge(err) {
					errorResponse(c, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body too large")
				} else {
					errorResponse(c, http.StatusBadRequest, "invalid_body", "Could not read request body")
				}
				c.Abort()
				return
			}
			if maxDepth > 0 && len(bytes.TrimSpace(data)) > 0 {
				if err := apiutil.ValidateJSONDepth(bytes.NewReader(data), maxDepth); err != nil {
					if errors.Is(err, apiutil.ErrJSONTooDeep) {
						errorResponse(c, http.StatusBadRequest, "json_too_deep", "JSON nesting too deep")
					} else {
						errorResponse(c, http.StatusBadRequest, "invalid_json", "Invalid JSON")
					}
					c.Abort()
					return
				}
			}
			c.Request.Body = io.NopCloser(bytes.NewReader(data))
			c.Request.ContentLength = int64(len(data))
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, body, maxBytes)
		c.Next()
	}
}

func isRequestBodyTooLarge(err error) bool {
	return err != nil && strings.Contains(err.Error(), "request body too large")
}
