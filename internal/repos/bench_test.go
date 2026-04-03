package repos

import "testing"

// Explorer-style Redis keys (same concatenation pattern as handlers; see Prefix* in keys.go).

var benchKeySink string

func BenchmarkExplorerCacheKeys_AddressTxBlock(b *testing.B) {
	const (
		addr   = "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
		txid   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		height = "840000"
	)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchKeySink = PrefixAddressCache + addr
		benchKeySink = PrefixTxCache + txid
		benchKeySink = PrefixBlockCache + height
	}
}

func BenchmarkPortfolioKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = PortfolioKey("alice", "pf-001")
	}
}

func BenchmarkWatchlistKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = WatchlistKey("alice", "wl-001")
	}
}

func BenchmarkSessionKey_CSRFKey(b *testing.B) {
	const sid = "sess_0123456789abcdef"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = SessionKey(sid)
		_ = CSRFKey(sid)
	}
}
