package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	resty "github.com/go-resty/resty/v2"
)

func TestBlockchairRequest_APIErrorStatus(t *testing.T) {
	skipIfRedisUnavailable(t)
	resetCache()
	// server returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("oops"))
	}))
	defer ts.Close()

	oldBase := baseURL
	oldKey := apiKey
	oldClient := httpClient
	defer func() { baseURL = oldBase; apiKey = oldKey; SetHTTPClient(oldClient) }()

	baseURL = ts.URL
	apiKey = "test-key"
	SetHTTPClient(resty.New().SetTimeout(2 * time.Second))

	_, err := blockchairRequest("getblockcount", []interface{}{})
	if err == nil {
		t.Fatalf("expected error for 500 response")
	}
}

func TestBlockchairRequest_InvalidJSON_PropagatesToGetBlockDetails(t *testing.T) {
	skipIfRedisUnavailable(t)
	resetCache()
	// server returns 200 but invalid JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer ts.Close()

	oldBase := baseURL
	oldKey := apiKey
	oldClient := httpClient
	defer func() { baseURL = oldBase; apiKey = oldKey; SetHTTPClient(oldClient) }()

	baseURL = ts.URL
	apiKey = "test-key"
	SetHTTPClient(resty.New().SetTimeout(2 * time.Second))

	_, err := getBlockDetails("1")
	if err == nil {
		t.Fatalf("expected json unmarshal error from getBlockDetails when API returns invalid JSON")
	}
}

func TestGetAddressDetails_CachedBytes(t *testing.T) {
	skipIfRedisUnavailable(t)
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
	skipIfRedisUnavailable(t)
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
	skipIfRedisUnavailable(t)
	resetCache()
	txid := "deadbeef"
	// store invalid JSON bytes in cache
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
	skipIfRedisUnavailable(t)
	resetCache()
	txid := "deadbeef"
	// store invalid JSON bytes in Redis cache
	rdb.Set(context.Background(), "tx:"+txid, []byte("not-json"), 0)

	_, err := getTransactionDetails(txid)
	if err == nil {
		t.Fatalf("expected error when cached bytes are invalid JSON")
	}
	if !IsInvalidCachedJSON(err) {
		t.Fatalf("expected InvalidCachedJSONError, got: %v", err)
	}
}

func TestBlockchairRequest_TimeoutRetryBehavior(t *testing.T) {
	skipIfRedisUnavailable(t)
	resetCache()
	// Server will delay response to trigger timeout
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"ok":true}}`))
	}))
	defer ts.Close()

	oldBase := baseURL
	oldKey := apiKey
	oldClient := httpClient
	defer func() { baseURL = oldBase; apiKey = oldKey; SetHTTPClient(oldClient) }()

	baseURL = ts.URL
	apiKey = "test-key"
	// Set a client with short timeout so request fails
	SetHTTPClient(resty.New().SetTimeout(50 * time.Millisecond).SetRetryCount(1))

	_, err := blockchairRequest("someMethod", []interface{}{})
	if err == nil {
		t.Fatalf("expected timeout error from blockchairRequest when server delays beyond timeout")
	}
}

func TestBlockchairRequest_RetryBehavior_SucceedsAfterNFailures(t *testing.T) {
	skipIfRedisUnavailable(t)
	resetCache()
	// This server will fail first 2 requests with 500, then succeed
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"ok":true}}`))
	}))
	defer ts.Close()

	oldBase := baseURL
	oldKey := apiKey
	oldClient := httpClient
	defer func() { baseURL = oldBase; apiKey = oldKey; SetHTTPClient(oldClient) }()

	baseURL = ts.URL
	apiKey = "test-key"
	// Set client to retry 3 times so that after 2 failures the 3rd attempt succeeds
	SetHTTPClient(resty.New().SetTimeout(1 * time.Second).SetRetryCount(3))

	resp, err := blockchairRequest("someMethod", []interface{}{})
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
