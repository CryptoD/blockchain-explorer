package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/andybalholm/brotli"
)

// ~24 KiB JSON payload (same order of magnitude as a heavy list-style API response).
var benchCompressPayload = func() []byte {
	type row struct {
		ID    int       `json:"id"`
		Name  string    `json:"name"`
		Price float64   `json:"price"`
		Extra []float64 `json:"extra"`
	}
	rows := make([]row, 600)
	for i := range rows {
		rows[i] = row{
			ID:    i,
			Name:  fmt.Sprintf("sym_%d", i),
			Price: float64(i) * 1.37,
			Extra: []float64{float64(i), float64(i + 1), float64(i + 2)},
		}
	}
	b, _ := json.Marshal(rows)
	return b
}()

// BenchmarkCompressLargeJSON_* measures CPU cost of compressing a typical JSON blob.
// Run: go test ./internal/server -bench=BenchmarkCompressLargeJSON -benchmem -run=^$
//
// On typical hardware, brotli at default quality is often several times slower than gzip default
// for similar input, with modestly smaller output. Prefer gzip when CPU-bound; prefer brotli when
// bandwidth is the bottleneck (or offload to a CDN / reverse proxy).
func BenchmarkCompressLargeJSON_gzipDefault(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := 0; i < b.N; i++ {
		buf.Reset()
		gw, _ := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
		_, _ = gw.Write(benchCompressPayload)
		_ = gw.Close()
	}
}

func BenchmarkCompressLargeJSON_brotliDefault(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := 0; i < b.N; i++ {
		buf.Reset()
		bw := brotli.NewWriterLevel(&buf, brotli.DefaultCompression)
		_, _ = bw.Write(benchCompressPayload)
		_ = bw.Close()
	}
}

func BenchmarkCompressLargeJSON_brotliBestSpeed(b *testing.B) {
	b.ReportAllocs()
	var buf bytes.Buffer
	for i := 0; i < b.N; i++ {
		buf.Reset()
		bw := brotli.NewWriterLevel(&buf, brotli.BestSpeed)
		_, _ = bw.Write(benchCompressPayload)
		_ = bw.Close()
	}
}
