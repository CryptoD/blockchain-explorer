package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
)

// RequestFingerprint is a stable string for the logical request (method + URI including sorted query).
func RequestFingerprint(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	q := r.URL.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(v)
			b.WriteByte('&')
		}
	}
	uri := r.URL.Path
	if b.Len() > 0 {
		uri += "?" + strings.TrimSuffix(b.String(), "&")
	}
	return r.Method + " " + uri
}

// RedisKey returns a Redis key for idempotency storage.
func RedisKey(scope, clientKey, fingerprint string) string {
	h := sha256.Sum256([]byte(strings.Join([]string{scope, clientKey, fingerprint}, "\x1e")))
	return "idempotency:v1:" + hex.EncodeToString(h[:])
}
