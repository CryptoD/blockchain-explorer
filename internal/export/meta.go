package export

// Version is embedded in JSON export payloads (search, portfolios list).
const Version = "1.0"

// CSV limits (blocks / transactions exports).
const (
	MaxBlockRange    = 500
	MaxBlockRows     = 2000
	MaxTxBlockRange  = 100
	MaxTxRows        = 5000
	DefaultBlockRows = 500
	DefaultTxRows    = 1000
)
