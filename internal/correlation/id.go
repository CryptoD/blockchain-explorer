// Package correlation generates and parses request/job correlation identifiers.
package correlation

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTP headers used to propagate correlation IDs (prefer Correlation-ID, then Request-ID).
const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
)

// MaxIDLength limits inbound propagated IDs.
const MaxIDLength = 128

// NewID returns a new opaque correlation identifier (32 hex characters).
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// FromHeaders returns the first non-empty inbound correlation or request ID, or empty if absent/invalid.
func FromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	for _, key := range []string{HeaderCorrelationID, HeaderRequestID} {
		v := strings.TrimSpace(h.Get(key))
		if v != "" && len(v) <= MaxIDLength {
			return v
		}
	}
	return ""
}
