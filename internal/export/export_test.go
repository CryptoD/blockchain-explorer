package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestFormatFloat(t *testing.T) {
	if s := FormatFloat(1.5); s == "" {
		t.Fatal("expected non-empty")
	}
}

func TestWriteBlocksCSVHeader(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := WriteBlocksCSVHeader(w); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if !strings.Contains(buf.String(), "height") {
		t.Fatalf("unexpected: %q", buf.String())
	}
}

func TestWritePortfolioHoldingsCSV_noResolver(t *testing.T) {
	var buf bytes.Buffer
	p := &PortfolioSnapshot{
		Name:    "Test",
		Created: time.Unix(1, 0).UTC(),
		Updated: time.Unix(2, 0).UTC(),
		Items: []PortfolioItemSnapshot{
			{Label: "L", Type: "crypto", Address: "", Symbol: "btc", Amount: 1},
		},
	}
	_ = WritePortfolioHoldingsCSV(context.Background(), &buf, "usd", 1, p.Created, p.Updated, p, nil)
	if !strings.Contains(buf.String(), "L") {
		t.Fatalf("expected row: %q", buf.String())
	}
}
