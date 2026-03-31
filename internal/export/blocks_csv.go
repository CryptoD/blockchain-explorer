package export

import (
	"encoding/csv"
	"fmt"
	"time"
)

// WriteBlocksCSVHeader writes the blocks export column row.
func WriteBlocksCSVHeader(w *csv.Writer) error {
	return w.Write([]string{"height", "hash", "time", "time_iso", "tx_count", "size", "weight", "difficulty", "confirmations"})
}

// WriteBlockRow writes one block data row.
func WriteBlockRow(w *csv.Writer, height int, hash string, timeVal float64, tm time.Time, txCount int, size, weight, difficulty float64, confs int) error {
	return w.Write([]string{
		fmt.Sprintf("%d", height),
		hash,
		fmt.Sprintf("%.0f", timeVal),
		tm.Format(time.RFC3339),
		fmt.Sprintf("%d", txCount),
		fmt.Sprintf("%.0f", size),
		fmt.Sprintf("%.0f", weight),
		fmt.Sprintf("%.0f", difficulty),
		fmt.Sprintf("%d", confs),
	})
}
