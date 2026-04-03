package export

// Version is embedded in JSON export payloads (search, portfolios list).
const Version = "1.0"

// CSV limits (blocks / transactions exports). Optional stricter caps: EXPORT_MAX_BLOCK_CSV_ROWS,
// EXPORT_MAX_TRANSACTION_CSV_ROWS (see docs/INPUT_LIMITS.md).
const (
	MaxBlockRange    = 500
	MaxBlockRows     = 2000
	MaxTxBlockRange  = 100
	MaxTxRows        = 5000
	DefaultBlockRows = 500
	DefaultTxRows    = 1000
)
