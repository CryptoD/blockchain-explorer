package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	resty "github.com/go-resty/resty/v2"
)

func TestCallBlockchain_APIErrorStatus(t *testing.T) {
	resetCache()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	}))
	defer ts.Close()

	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", resty.New().SetTimeout(2*time.Second)))
	defer SetBlockchainClient(nil)

	_, err := callBlockchain(context.Background(), "getblockcount", []interface{}{})
	if err == nil {
		t.Fatalf("expected error for 500 response")
	}
}

func TestGetBlockDetails_InvalidJSON(t *testing.T) {
	resetCache()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer ts.Close()

	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", resty.New().SetTimeout(2*time.Second)))
	defer SetBlockchainClient(nil)

	_, err := getBlockDetails("1")
	if err == nil {
		t.Fatalf("expected json unmarshal error from getBlockDetails when API returns invalid JSON")
	}
}

func TestGetAddressDetails_CachedBytes(t *testing.T) {
	resetCache()
	addr := "1ExampleAddr"
	obj := map[string]interface{}{"result": map[string]interface{}{"address": addr}}
	b, _ := json.Marshal(obj)
	rdb.Set(context.Background(), "address:"+addr, b, 0)

	res, err := getAddressDetails(addr)
	if err != nil {
		t.Fatalf("expected no error when cached bytes present, got %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestGetTransactionDetails_CachedBytes_Behavior(t *testing.T) {
	resetCache()
	txid := "b0c0"
	obj := map[string]interface{}{"hash": txid}
	b, _ := json.Marshal(obj)
	rdb.Set(context.Background(), "tx:"+txid, b, 0)

	res, err := getTransactionDetails(txid)
	if err != nil {
		t.Fatalf("expected no error when tx cache contains []byte after changed behavior, got %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestGetTransactionDetails_CachedBytes_InvalidJSON_ReturnsError(t *testing.T) {
	resetCache()
	txid := "deadbeef"
	rdb.Set(context.Background(), "tx:"+txid, "not-json", 0)

	res, err := getTransactionDetails(txid)
	if err == nil {
		t.Fatalf("expected error when cached bytes are invalid JSON, got nil and res=%v", res)
	}
	if err != nil && !strings.Contains(err.Error(), "invalid cached JSON") {
		t.Fatalf("expected invalid cached JSON error, got: %v", err)
	}
}

func TestGetTransactionDetails_CachedBytes_InvalidJSON_ReturnsCustomError(t *testing.T) {
	resetCache()
	txid := "deadbeef"
	rdb.Set(context.Background(), "tx:"+txid, []byte("not-json"), 0)

	_, err := getTransactionDetails(txid)
	if err == nil {
		t.Fatalf("expected error when cached bytes are invalid JSON")
	}
	if !IsInvalidCachedJSON(err) {
		t.Fatalf("expected InvalidCachedJSONError, got: %v", err)
	}
}

func TestCallBlockchain_TimeoutRetryBehavior(t *testing.T) {
	resetCache()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"ok":true}}`))
	}))
	defer ts.Close()

	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", resty.New().SetTimeout(50*time.Millisecond).SetRetryCount(1)))
	defer SetBlockchainClient(nil)

	_, err := callBlockchain(context.Background(), "someMethod", []interface{}{})
	if err == nil {
		t.Fatalf("expected timeout error from callBlockchain when server delays beyond timeout")
	}
}

func TestCallBlockchain_RetrySucceedsAfterFailures(t *testing.T) {
	resetCache()
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"ok":true}}`))
	}))
	defer ts.Close()

	rc := resty.New().
		SetTimeout(1 * time.Second).
		SetRetryCount(3).
		AddRetryAfterErrorCondition() // retry on 5xx; default resty only retries transport errors
	SetBlockchainClient(blockchain.NewGetBlockRPCClient(ts.URL, "test-key", rc))
	defer SetBlockchainClient(nil)

	resp, err := callBlockchain(context.Background(), "someMethod", []interface{}{})
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected a response after retries succeeded")
	}
	if callCount <= 2 {
		t.Fatalf("expected callCount > 2 since retries should have been attempted; got %d", callCount)
	}
}
