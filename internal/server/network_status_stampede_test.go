package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	resty "github.com/go-resty/resty/v2"
)

func TestGetNetworkStatus_concurrentMissSingleFlight(t *testing.T) {
	resetCache()
	var rpcCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rpcCalls.Add(1)
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "getblockcount":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":800000}`))
		case "getdifficulty":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":123.45}`))
		case "getnetworkhashps":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":1.5e12}`))
		default:
			http.Error(w, "unknown method", http.StatusBadRequest)
		}
	}))
	defer ts.Close()

	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", resty.New().SetTimeout(5*time.Second)))
	defer SetBlockchainClient(nil)

	const n = 40
	var wg sync.WaitGroup
	var fail atomic.Int32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			m, err := getNetworkStatus()
			if err != nil {
				fail.Store(1)
				return
			}
			bh, ok := m["block_height"].(float64)
			if !ok || bh != 800000 {
				fail.Store(1)
			}
		}()
	}
	wg.Wait()
	if fail.Load() != 0 {
		t.Fatal("concurrent getNetworkStatus failures or bad payload")
	}
	// One coalesced rebuild issues 3 JSON-RPC POSTs.
	if rpcCalls.Load() != 3 {
		t.Fatalf("expected 3 RPC round-trips (one rebuild), got %d", rpcCalls.Load())
	}
}
