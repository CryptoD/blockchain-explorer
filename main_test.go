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
	"github.com/gin-gonic/gin"
	resty "github.com/go-resty/resty/v2"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	SetBlockchainClient(nil)
	SetPricingClient(nil)
	os.Exit(m.Run())
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

// skipIfRedisUnavailable skips the test if Redis is not running (e.g. in CI without a service).
func skipIfRedisUnavailable(t *testing.T) {
	t.Helper()
	if rdb == nil {
		t.Skip("Redis client not configured")
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
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
	skipIfRedisUnavailable(t)
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
	skipIfRedisUnavailable(t)
	resetCache()

	// Address
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

	// Transaction
	// Use a valid 64-char hex txid
	txid := strings.Repeat("b", 60) + "0a0b"
	setCache("tx:"+txid, map[string]interface{}{"hash": txid})
	typeStr, res, err = searchBlockchain(txid)
	if err != nil {
		t.Fatalf("searchBlockchain(tx) returned error: %v", err)
	}
	if typeStr != "transaction" {
		t.Fatalf("expected type 'transaction', got %s", typeStr)
	}

	// Block height
	height := "12345"
	setCache("block:"+height, map[string]interface{}{"result": map[string]interface{}{"height": 12345}})
	typeStr, res, err = searchBlockchain(height)
	if err != nil {
		t.Fatalf("searchBlockchain(block) returned error: %v", err)
	}
	if typeStr != "block" {
		t.Fatalf("expected type 'block', got %s", typeStr)
	}

	// Not found
	_, _, err = searchBlockchain("definitely-not-a-valid-query-!@#")
	if err == nil {
		t.Fatalf("expected ErrNotFound for unknown query")
	}
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFetchLatestBlocksAndTransactions(t *testing.T) {
	skipIfRedisUnavailable(t)
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

	// Test fetchLatestBlocks
	blocks, err := fetchLatestBlocks(3)
	if err != nil {
		t.Fatalf("fetchLatestBlocks returned error: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}

	// Test fetchLatestTransactions: want up to 3 txs
	txs, err := fetchLatestTransactions(2, 3)
	if err != nil {
		t.Fatalf("fetchLatestTransactions returned error: %v", err)
	}
	if len(txs) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(txs))
	}
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
	skipIfRedisUnavailable(t)
	resetCache()
	SetBlockchainClient(blockchain.NewGetBlockRPCClient("", "", resty.New()))
	defer SetBlockchainClient(nil)
	_, err := callBlockchain(context.Background(), "someMethod", []interface{}{})
	if err == nil {
		t.Fatalf("expected error when RPC client not configured")
	}
}

func TestCallBlockchain_SuccessWithTestServer(t *testing.T) {
	skipIfRedisUnavailable(t)
	resetCache()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		resp := map[string]interface{}{"jsonrpc": "2.0", "id": payload["id"], "result": map[string]interface{}{"ok": true}}
		respBytes, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(respBytes)
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
	skipIfRedisUnavailable(t)
	resetCache()
	// set Gin to test mode to avoid noisy output
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/api/search", searchHandler)

	// seed an address into cache
	addr := "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
	addrData := map[string]interface{}{"result": map[string]interface{}{"address": addr}}
	setCache("address:"+addr, addrData)

	// Perform a request
	req := httptest.NewRequest("GET", "/api/search?q="+addr, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// Check headers
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("expected ETag header to be set")
	}
	if cacheControl := w.Header().Get("Cache-Control"); cacheControl == "" {
		t.Fatalf("expected Cache-Control header to be set")
	}

	// Verify response body contains type and result
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

	// Now test If-None-Match handling: send same ETag and expect 304
	req2 := httptest.NewRequest("GET", "/api/search?q="+addr, nil)
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != 304 {
		t.Fatalf("expected 304 Not Modified, got %d", w2.Code)
	}

	// Missing query param -> 400
	req3 := httptest.NewRequest("GET", "/api/search", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != 400 {
		t.Fatalf("expected 400 for missing query, got %d", w3.Code)
	}
}

// TestAdvancedSearchHandler tests the advanced symbol search with filters and sorting
func TestAdvancedSearchHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/search/advanced", advancedSearchHandler)
	router.GET("/api/search/categories", getSymbolCategoriesHandler)

	// Test 1: Basic search without filters
	req1 := httptest.NewRequest("GET", "/api/search/advanced", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("expected 200 for basic search, got %d", w1.Code)
	}
	var result1 map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &result1); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if _, ok := result1["data"]; !ok {
		t.Fatalf("expected 'data' field in response")
	}
	if _, ok := result1["pagination"]; !ok {
		t.Fatalf("expected 'pagination' field in response")
	}

	// Test 2: Search with query
	req2 := httptest.NewRequest("GET", "/api/search/advanced?q=BTC", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("expected 200 for search with query, got %d", w2.Code)
	}

	// Test 3: Search with type filter
	req3 := httptest.NewRequest("GET", "/api/search/advanced?types=crypto", nil)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("expected 200 for search with type filter, got %d", w3.Code)
	}

	// Test 4: Search with category filter
	req4 := httptest.NewRequest("GET", "/api/search/advanced?categories=defi", nil)
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("expected 200 for search with category filter, got %d", w4.Code)
	}

	// Test 5: Search with sorting
	req5 := httptest.NewRequest("GET", "/api/search/advanced?sort_by=price&sort_dir=desc", nil)
	w5 := httptest.NewRecorder()
	router.ServeHTTP(w5, req5)
	if w5.Code != 200 {
		t.Fatalf("expected 200 for search with sorting, got %d", w5.Code)
	}

	// Test 6: Search with price range
	req6 := httptest.NewRequest("GET", "/api/search/advanced?min_price=1&max_price=1000", nil)
	w6 := httptest.NewRecorder()
	router.ServeHTTP(w6, req6)
	if w6.Code != 200 {
		t.Fatalf("expected 200 for search with price range, got %d", w6.Code)
	}

	// Test 7: Search with pagination
	req7 := httptest.NewRequest("GET", "/api/search/advanced?page=1&page_size=5", nil)
	w7 := httptest.NewRecorder()
	router.ServeHTTP(w7, req7)
	if w7.Code != 200 {
		t.Fatalf("expected 200 for search with pagination, got %d", w7.Code)
	}

	// Test 8: Get categories
	req8 := httptest.NewRequest("GET", "/api/search/categories", nil)
	w8 := httptest.NewRecorder()
	router.ServeHTTP(w8, req8)
	if w8.Code != 200 {
		t.Fatalf("expected 200 for categories endpoint, got %d", w8.Code)
	}
	var result8 map[string][]string
	if err := json.Unmarshal(w8.Body.Bytes(), &result8); err != nil {
		t.Fatalf("failed to parse categories response: %v", err)
	}
	if _, ok := result8["types"]; !ok {
		t.Fatalf("expected 'types' field in categories response")
	}
	if _, ok := result8["categories"]; !ok {
		t.Fatalf("expected 'categories' field in categories response")
	}
}

// TestMatchesFilters tests the filter matching logic
func TestMatchesFilters(t *testing.T) {
	symbol := SymbolInfo{
		Symbol:   "BTC",
		Name:     "Bitcoin",
		Type:     "crypto",
		Category: "layer1",
		Price:    45000.0,
		MarketCap: 850000000000,
		IsActive: true,
	}

	// Test type filter match
	filters1 := SearchFilters{Types: []string{"crypto"}}
	if !matchesFilters(symbol, filters1) {
		t.Error("expected symbol to match crypto type filter")
	}

	// Test type filter no match
	filters2 := SearchFilters{Types: []string{"stock"}}
	if matchesFilters(symbol, filters2) {
		t.Error("expected symbol not to match stock type filter")
	}

	// Test category filter match
	filters3 := SearchFilters{Categories: []string{"layer1"}}
	if !matchesFilters(symbol, filters3) {
		t.Error("expected symbol to match layer1 category filter")
	}

	// Test price range filter
	filters4 := SearchFilters{MinPrice: 1000, MaxPrice: 50000}
	if !matchesFilters(symbol, filters4) {
		t.Error("expected symbol to match price range filter")
	}

	// Test price range filter - too high
	filters5 := SearchFilters{MinPrice: 50000}
	if matchesFilters(symbol, filters5) {
		t.Error("expected symbol not to match min price filter")
	}

	// Test market cap filter
	filters6 := SearchFilters{MinMarketCap: 1000000000}
	if !matchesFilters(symbol, filters6) {
		t.Error("expected symbol to match market cap filter")
	}

	// Test active status filter
	isActive := true
	filters7 := SearchFilters{IsActive: &isActive}
	if !matchesFilters(symbol, filters7) {
		t.Error("expected symbol to match active status filter")
	}

	isActive = false
	filters8 := SearchFilters{IsActive: &isActive}
	if matchesFilters(symbol, filters8) {
		t.Error("expected symbol not to match inactive status filter")
	}
}

// TestSortSymbols tests the sorting logic
func TestSortSymbols(t *testing.T) {
	symbols := []SymbolInfo{
		{Symbol: "ETH", Price: 2300, Rank: 2},
		{Symbol: "BTC", Price: 45000, Rank: 1},
		{Symbol: "SOL", Price: 98, Rank: 3},
	}

	// Test sort by rank ascending
	sort1 := SortOptions{Field: "rank", Direction: "asc"}
	sortSymbols(symbols, sort1)
	if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
		t.Error("expected symbols sorted by rank ascending")
	}

	// Test sort by price descending
	sort2 := SortOptions{Field: "price", Direction: "desc"}
	sortSymbols(symbols, sort2)
	if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
		t.Error("expected symbols sorted by price descending")
	}

	// Test sort by symbol ascending
	sort3 := SortOptions{Field: "symbol", Direction: "asc"}
	sortSymbols(symbols, sort3)
	if symbols[0].Symbol != "BTC" || symbols[1].Symbol != "ETH" || symbols[2].Symbol != "SOL" {
		t.Error("expected symbols sorted by symbol ascending")
	}
}
