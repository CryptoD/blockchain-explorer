package server

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/CryptoD/blockchain-explorer/internal/config"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

// Extensions that are already compressed or rarely benefit from on-the-fly gzip/brotli.
var compressionExcludedExt = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".ico": {},
	".woff": {}, ".woff2": {}, ".br": {}, ".gz": {}, ".zip": {},
	".mp4": {}, ".webm": {}, ".mp3": {}, ".pdf": {},
}

func compressionExcludedPrefixes(path string) bool {
	if strings.HasPrefix(path, "/debug/pprof") {
		return true
	}
	// PDF is already compressed internally; wrapping wastes CPU for little gain.
	if strings.HasSuffix(path, "/export/pdf") {
		return true
	}
	return false
}

func shouldSkipResponseCompression(c *gin.Context) bool {
	path := c.Request.URL.Path
	if compressionExcludedPrefixes(path) {
		return true
	}
	ext := filepath.Ext(path)
	if _, ok := compressionExcludedExt[ext]; ok {
		return true
	}
	if strings.Contains(strings.ToLower(c.Request.Header.Get("Connection")), "upgrade") {
		return true
	}
	return false
}

// negotiateWriteCloser picks br (if allowed), gzip, or identity based on Accept-Encoding.
func negotiateWriteCloser(w http.ResponseWriter, r *http.Request, brotliOK bool) io.WriteCloser {
	if brotliOK {
		return brotli.HTTPCompressor(w, r)
	}
	return gzipOnlyWriteCloser(w, r)
}

type gzipIdentityWriter struct {
	http.ResponseWriter
}

func (gzipIdentityWriter) Close() error { return nil }

func gzipOnlyWriteCloser(w http.ResponseWriter, r *http.Request) io.WriteCloser {
	if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return gzipIdentityWriter{w}
	}
	ae := r.Header.Get("Accept-Encoding")
	if !strings.Contains(ae, "gzip") {
		return gzipIdentityWriter{w}
	}
	if w.Header().Get("Vary") == "" {
		w.Header().Set("Vary", "Accept-Encoding")
	} else {
		w.Header().Add("Vary", "Accept-Encoding")
	}
	w.Header().Set("Content-Encoding", "gzip")
	return gzip.NewWriter(w)
}

// compressingGinWriter wraps a gin ResponseWriter and compresses the body (gzip or brotli).
// Pattern aligned with github.com/gin-contrib/gzip (Hijack, Flush, Content-Length stripping).
type compressingGinWriter struct {
	gin.ResponseWriter
	enc  io.WriteCloser
	size int
}

func (w *compressingGinWriter) WriteString(s string) (int, error) {
	w.Header().Del("Content-Length")
	n, err := w.enc.Write([]byte(s))
	w.size += n
	return n, err
}

func (w *compressingGinWriter) Write(data []byte) (int, error) {
	w.Header().Del("Content-Length")
	n, err := w.enc.Write(data)
	w.size += n
	return n, err
}

func (w *compressingGinWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func (w *compressingGinWriter) Flush() {
	if f, ok := w.enc.(interface{ Flush() error }); ok {
		_ = f.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *compressingGinWriter) Size() int {
	return w.size
}

var _ http.Hijacker = (*compressingGinWriter)(nil)

func (w *compressingGinWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

// responseCompressionMiddleware compresses JSON/HTML/text responses when the client accepts gzip or brotli.
func responseCompressionMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || !cfg.ResponseCompressionEnabled {
			c.Next()
			return
		}
		if shouldSkipResponseCompression(c) {
			c.Next()
			return
		}

		orig := c.Writer
		enc := negotiateWriteCloser(orig, c.Request, cfg.ResponseCompressionBrotli)
		defer func() { _ = enc.Close() }()

		c.Writer = &compressingGinWriter{
			ResponseWriter: orig,
			enc:            enc,
		}
		c.Next()
	}
}
