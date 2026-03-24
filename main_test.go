package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/CryptoD/blockchain-explorer/internal/pricing"
	"github.com/CryptoD/blockchain-explorer/internal/redistest"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	resty "github.com/go-resty/resty/v2"
	"github.com/redis/go-redis/v9"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	SetBlockchainClient(nil)
	SetPricingClient(nil)

	if redistest.UseIntegration() {
		old := rdb
		cl := redis.NewClient(&redis.Options{
			Addr:            redistest.IntegrationAddr(),
			DisableIdentity: true,
		})
		rdb = cl
		if err := cl.Ping(ctx).Err(); err != nil {
			fmt.Fprintf(os.Stderr, "integration Redis at %s: %v\n", redistest.IntegrationAddr(), err)
			os.Exit(1)
		}
		code := m.Run()
		_ = cl.Close()
		rdb = old
		os.Exit(code)
	}

	mr, err := miniredis.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "miniredis: %v\n", err)
		os.Exit(1)
	}
	old := rdb
	cl := redis.NewClient(&redis.Options{
		Addr:            mr.Addr(),
		DisableIdentity: true,
	})
	rdb = cl
	code := m.Run()
	_ = cl.Close()
	mr.Close()
	rdb = old
	os.Exit(code)
}

// TestSetPricingClient_MockNoNetwork ensures pricing paths can use pricing.MockClient
// (no CoinGecko HTTP) when injected via SetPricingClient.
func TestSetPricingClient_MockNoNetwork(t *testing.T) {
	m := &pricing.MockClient{
		GetMultiCurrencyRatesInFunc: func(ctx context.Context, currencies []string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"bitcoin": map[string]interface{}{"usd": 50000.0},
			}, nil
		},
		GetBTCUSDFunc: func(ctx context.Context) (float64, error) {
			return 50000, nil
		},
	}
	SetPricingClient(m)
	defer SetPricingClient(nil)
	rates, err := pricingClient.GetMultiCurrencyRatesIn(context.Background(), []string{"usd"})
	if err != nil {
		t.Fatal(err)
	}
	btc, ok := rates["bitcoin"].(map[string]interface{})
	if !ok || btc["usd"] != 50000.0 {
		t.Fatalf("rates = %#v", rates)
	}
	usd, err := pricingClient.GetBTCUSD(context.Background())
	if err != nil || usd != 50000 {
		t.Fatalf("GetBTCUSD = %v, %v", usd, err)
	}
}

// helper to reset global cache between tests
func resetCache() {
	rdb.FlushDB(ctx)
}

// helper to set cache for tests
func setCache(key string, value interface{}) {
	data, _ := json.Marshal(value)
	rdb.Set(ctx, key, data, 0)
}

func TestValidationFunctions(t *testing.T) {
	resetCache()

	// isValidAddress
	addrTests := []struct {
		addr string
		exp  bool
	}{
		{"1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", true},
		{"3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", true},
		{"bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kygt080", true},
		{"mzBc4XEFSdzCDcTxAgf6EZXgsZWpztRhe", false}, // testnet (starts with m) is not accepted by this simple validator
		{"x1invalid", false},
		{"1short", false},
	}

	for _, tt := range addrTests {
		res := isValidAddress(tt.addr)
		if res != tt.exp {
			t.Fatalf("isValidAddress(%q) = %v, want %v", tt.addr, res, tt.exp)
		}
	}

	// isValidTransactionID
	txGood := strings.Repeat("a", 64)
	if !isValidTransactionID(txGood) {
		t.Fatalf("expected valid txid for %s", txGood)
	}
	if isValidTransactionID("shorttx") {
		t.Fatalf("expected invalid txid for short string")
	}
	if isValidTransactionID(strings.Repeat("g", 64)) { // 'g' is not hex
		t.Fatalf("expected invalid txid for non-hex string")
	}

	// isValidBlockHeight
	if !isValidBlockHeight("12345") {
		t.Fatalf("expected valid block height for 12345")
	}
	if isValidBlockHeight("12x45") {
		t.Fatalf("expected invalid block height for 12x45")
	}
}

func TestSearchBlockchain_AddressTransactionBlockAndNotFound(t *testing.T) {
	resetCache()

	t.Run("address", func(t *testing.T) {
		addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
		setCache("address:"+addr, map[string]interface{}{"result": map[string]interface{}{"address": addr}})
		typeStr, res, err := searchBlockchain(addr)
		if err != nil {
			t.Fatalf("searchBlockchain(address) returned error: %v", err)
		}
		if typeStr != "address" {
			t.Fatalf("expected type 'address', got %s", typeStr)
		}
		if res == nil {
			t.Fatalf("expected non-nil result for address search")
		}
	})

	t.Run("transaction", func(t *testing.T) {
		txid := strings.Repeat("b", 60) + "0a0b"
		setCache("tx:"+txid, map[string]interface{}{"hash": txid})
		typeStr, _, err := searchBlockchain(txid)
		if err != nil {
			t.Fatalf("searchBlockchain(tx) returned error: %v", err)
		}
		if typeStr != "transaction" {
			t.Fatalf("expected type 'transaction', got %s", typeStr)
		}
	})

	t.Run("block_height", func(t *testing.T) {
		height := "12345"
		setCache("block:"+height, map[string]interface{}{"result": map[string]interface{}{"height": 12345}})
		typeStr, _, err := searchBlockchain(height)
		if err != nil {
			t.Fatalf("searchBlockchain(block) returned error: %v", err)
		}
		if typeStr != "block" {
			t.Fatalf("expected type 'block', got %s", typeStr)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, _, err := searchBlockchain("definitely-not-a-valid-query-!@#")
		if err == nil {
			t.Fatalf("expected ErrNotFound for unknown query")
		}
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestFetchLatestBlocksAndTransactions(t *testing.T) {
	resetCache()

	// Seed network status with best_block_height = 100
	network := map[string]interface{}{"result": map[string]interface{}{"best_block_height": float64(100)}}
	setCache("network:status", network)

	// Seed blocks 100, 99, 98 with tx lists
	for i := 0; i < 3; i++ {
		h := 100 - i
		tx1 := fmtTxID(h, 1)
		tx2 := fmtTxID(h, 2)
		block := map[string]interface{}{"result": map[string]interface{}{"height": h, "tx": []interface{}{tx1, tx2}}}
		setCache(fmtBlockKey(h), block)
		setCache(fmtTxKey(tx1), map[string]interface{}{"hash": tx1})
		setCache(fmtTxKey(tx2), map[string]interface{}{"hash": tx2})
	}

	t.Run("fetchLatestBlocks", func(t *testing.T) {
		blocks, err := fetchLatestBlocks(3)
		if err != nil {
			t.Fatalf("fetchLatestBlocks returned error: %v", err)
		}
		if len(blocks) != 3 {
			t.Fatalf("expected 3 blocks, got %d", len(blocks))
		}
	})

	t.Run("fetchLatestTransactions", func(t *testing.T) {
		txs, err := fetchLatestTransactions(2, 3)
		if err != nil {
			t.Fatalf("fetchLatestTransactions returned error: %v", err)
		}
		if len(txs) != 3 {
			t.Fatalf("expected 3 transactions, got %d", len(txs))
		}
	})
}

// helper formatting functions to keep keys consistent with code
func fmtBlockKey(h int) string  { return "block:" + strconv.Itoa(h) }
func fmtTxKey(tx string) string { return "tx:" + tx }
func fmtTxID(h, idx int) string {
	// produce a deterministic 64-char hex-like txid (all 'a' then two hex bytes)
	base := strings.Repeat("a", 60)
	return base + fmt.Sprintf("%02x%02x", h%256, idx%256)
}

func TestRPCCall_UnconfiguredReturnsError(t *testing.T) {
	resetCache()
	SetBlockchainClient(blockchain.NewGetBlockRPCClient("", "", resty.New()))
	defer SetBlockchainClient(nil)
	_, err := callBlockchain(context.Background(), "someMethod", []interface{}{})
	if err == nil {
		t.Fatalf("expected error when RPC client not configured")
	}
}

func TestCallBlockchain_SuccessWithTestServer(t *testing.T) {
	resetCache()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": payload["id"], "result": map[string]interface{}{"ok": true}}
		respBytes, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(respBytes)
	}))
	defer ts.Close()

	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", resty.New().SetTimeout(2*time.Second)))
	defer SetBlockchainClient(nil)

	resp, err := callBlockchain(context.Background(), "someMethod", []interface{}{"param1"})
	if err != nil {
		t.Fatalf("expected no error from callBlockchain, got %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &parsed); err != nil {
		t.Fatalf("failed to unmarshal response body: %v", err)
	}
	result, ok := parsed["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result format")
	}
	if _, ok := result["ok"]; !ok {
		t.Fatalf("expected ok key in result")
	}
}

func TestSearchHandler_HTTPBehavior(t *testing.T) {
	resetCache()
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/api/search", searchHandler)

	addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	setCache("address:"+addr, map[string]interface{}{"result": map[string]interface{}{"address": addr}})

	var etag string
	t.Run("first_get_sets_etag_and_body", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search?q="+addr, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 OK, got %d", w.Code)
		}
		etag = w.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("expected ETag header to be set")
		}
		if w.Header().Get("Cache-Control") == "" {
			t.Fatalf("expected Cache-Control header to be set")
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}
		if body["type"] != "address" {
			t.Fatalf("expected type 'address', got %v", body["type"])
		}
		if _, ok := body["result"]; !ok {
			t.Fatalf("expected result in response body")
		}
	})

	t.Run("if_none_match_304", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search?q="+addr, nil)
		req.Header.Set("If-None-Match", etag)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 304 {
			t.Fatalf("expected 304 Not Modified, got %d", w.Code)
		}
	})

	t.Run("missing_query_400", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("expected 400 for missing query, got %d", w.Code)
		}
	})
}

func TestAdvancedSearchHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/search/advanced", advancedSearchHandler)
	router.GET("/api/search/categories", getSymbolCategoriesHandler)

	t.Run("basic_no_filters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for basic search, got %d", w.Code)
		}
		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if _, ok := result["data"]; !ok {
			t.Fatalf("expected 'data' field in response")
		}
		if _, ok := result["pagination"]; !ok {
			t.Fatalf("expected 'pagination' field in response")
		}
	})

	t.Run("query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?q=BTC", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with query, got %d", w.Code)
		}
	})

	t.Run("type_filter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?types=crypto", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with type filter, got %d", w.Code)
		}
	})

	t.Run("category_filter", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?categories=defi", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with category filter, got %d", w.Code)
		}
	})

	t.Run("sorting", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?sort_by=price&sort_dir=desc", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with sorting, got %d", w.Code)
		}
	})

	t.Run("price_range", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?min_price=1&max_price=1000", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with price range, got %d", w.Code)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/advanced?page=1&page_size=5", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for search with pagination, got %d", w.Code)
		}
	})

	t.Run("categories_endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/search/categories", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("expected 200 for categories endpoint, got %d", w.Code)
		}
		var result map[string][]string
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to parse categories response: %v", err)
		}
		if _, ok := result["types"]; !ok {
			t.Fatalf("expected 'types' field in categories response")
		}
		if _, ok := result["categories"]; !ok {
			t.Fatalf("expected 'categories' field in categories response")
		}
	})
}

func TestMatchesFilters(t *testing.T) {
	symbol := SymbolInfo{
		Symbol:    "BTC",
		Name:      "Bitcoin",
		Type:      "crypto",
		Category:  "layer1",
		Price:     45000.0,
		MarketCap: 850000000000,
		IsActive:  true,
	}

	t.Run("type_match", func(t *testing.T) {
		if !matchesFilters(symbol, SearchFilters{Types: []string{"crypto"}}) {
			t.Error("expected symbol to match crypto type filter")
		}
	})
	t.Run("type_no_match", func(t *testing.T) {
		if matchesFilters(symbol, SearchFilters{Types: []string{"stock"}}) {
			t.Error("expected symbol not to match stock type filter")
		}
	})
	t.Run("category_match", func(t *testing.T) {
		if !matchesFilters(symbol, SearchFilters{Categories: []string{"layer1"}}) {
			t.Error("expected symbol to match layer1 category filter")
		}
	})
	t.Run("price_range_match", func(t *testing.T) {
		if !matchesFilters(symbol, SearchFilters{MinPrice: 1000, MaxPrice: 50000}) {
			t.Error("expected symbol to match price range filter")
		}
	})
	t.Run("price_min_too_high", func(t *testing.T) {
		if matchesFilters(symbol, SearchFilters{MinPrice: 50000}) {
			t.Error("expected symbol not to match min price filter")
		}
	})
	t.Run("market_cap", func(t *testing.T) {
		if !matchesFilters(symbol, SearchFilters{MinMarketCap: 1000000000}) {
			t.Error("expected symbol to match market cap filter")
		}
	})
	t.Run("active_true", func(t *testing.T) {
		isActive := true
		if !matchesFilters(symbol, SearchFilters{IsActive: &isActive}) {
			t.Error("expected symbol to match active status filter")
		}
	})
	t.Run("active_false", func(t *testing.T) {
		isActive := false
		if matchesFilters(symbol, SearchFilters{IsActive: &isActive}) {
			t.Error("expected symbol not to match inactive status filter")
		}
	})
}

func TestSortSymbols(t *testing.T) {
	base := []SymbolInfo{
		{Symbol: "ETH", Price: 2300, Rank: 2},
		{Symbol: "BTC", Price: 45000, Rank: 1},
		{Symbol: "SOL", Price: 98, Rank: 3},
	}

	t.Run("rank_asc", func(t *testing.T) {
		symbols := append([]SymbolInfo(nil), base...)
		sortSymbols(symbols, SortOptions{Field: "rank", Direction: "asc"})
		if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
			t.Error("expected symbols sorted by rank ascending")
		}
	})
	t.Run("price_desc", func(t *testing.T) {
		symbols := append([]SymbolInfo(nil), base...)
		sortSymbols(symbols, SortOptions{Field: "price", Direction: "desc"})
		if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
			t.Error("expected symbols sorted by price descending")
		}
	})
	t.Run("symbol_asc", func(t *testing.T) {
		symbols := append([]SymbolInfo(nil), base...)
		sortSymbols(symbols, SortOptions{Field: "symbol", Direction: "asc"})
		if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
			t.Error("expected symbols sorted by symbol ascending")
		}
	})
}
