package server

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

// Blockchain search with a warm Redis cache (same setup as TestSearchBlockchain_AddressTransactionBlockAndNotFound).

func BenchmarkSearchBlockchain_AddressCacheHit(b *testing.B) {
	resetCache()
	addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	setCache("address:"+addr, map[string]interface{}{"result": map[string]interface{}{"address": addr}})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := searchBlockchain(addr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSearchBlockchain_TransactionCacheHit(b *testing.B) {
	resetCache()
	txid := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	setCache("tx:"+txid, map[string]interface{}{"hash": txid})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, err := searchBlockchain(txid)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// JSONMarshalSearchPayloadLarge mirrors searchHandler: marshal result map for ETag, plus a gin-style API envelope.

func BenchmarkJSONMarshalSearchPayloadLarge(b *testing.B) {
	const n = 2000
	result := make(map[string]interface{}, 2)
	txs := make([]map[string]interface{}, n)
	for i := range txs {
		txs[i] = map[string]interface{}{
			"txid": strconv.Itoa(i),
			"fee":  1250 + i,
			"size": 220 + (i % 500),
		}
	}
	result["address"] = "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	result["transactions"] = txs

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(result)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshalAdvancedSearchEnvelopeLarge(b *testing.B) {
	const n = 1000
	items := make([]SymbolInfo, n)
	base := symbolDatabase[0]
	for i := range items {
		items[i] = base
		items[i].Symbol = "SYM" + strconv.Itoa(i)
		items[i].Rank = i + 1
	}
	payload := gin.H{
		"data": items,
		"pagination": gin.H{
			"page": 1, "page_size": 100, "total": n, "total_pages": (n + 99) / 100,
		},
		"filters_applied": gin.H{},
		"sort_applied":    gin.H{"field": "rank", "direction": "asc"},
		"available_filters": gin.H{
			"types":      []string{"crypto"},
			"categories": []string{"layer1", "defi"},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}
