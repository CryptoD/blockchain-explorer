package export

import (
	"encoding/csv"
	"fmt"
)

// WriteTransactionsCSVHeader writes the transactions export column row.
func WriteTransactionsCSVHeader(w *csv.Writer) error {
	return w.Write([]string{"txid", "block_height", "block_hash", "block_time", "block_time_iso", "size", "vsize", "weight", "fee", "locktime", "version"})
}

// WriteTransactionRow writes one transaction data row.
func WriteTransactionRow(w *csv.Writer, txid string, blockHeight int, blockHash string, blockTime float64, blockTimeISO string, size, vsize, weight, fee, locktime, version float64) error {
	return w.Write([]string{
		txid,
		fmt.Sprintf("%d", blockHeight),
		blockHash,
		fmt.Sprintf("%.0f", blockTime),
		blockTimeISO,
		fmt.Sprintf("%.0f", size),
		fmt.Sprintf("%.0f", vsize),
		fmt.Sprintf("%.0f", weight),
		fmt.Sprintf("%.6f", fee),
		fmt.Sprintf("%.0f", locktime),
		fmt.Sprintf("%.0f", version),
	})
}
