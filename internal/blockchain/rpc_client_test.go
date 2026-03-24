package blockchain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CryptoD/blockchain-explorer/internal/blockchain"
	"github.com/go-resty/resty/v2"
)

func TestGetBlockRPCClient_Call_Unconfigured(t *testing.T) {
	c := blockchain.NewGetBlockRPCClient("", "", resty.New())
	_, err := c.Call(context.Background(), "getblockcount", nil)
	if err == nil {
		t.Fatal("expected error when base URL or API key missing")
	}
}

func TestMockRPCClient_Call_DefaultError(t *testing.T) {
	var m *blockchain.MockRPCClient
	_, err := m.Call(context.Background(), "x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockRPCClient_Call_Custom(t *testing.T) {
	want := errors.New("boom")
	m := &blockchain.MockRPCClient{
		CallFunc: func(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
			if method != "getblockcount" {
				t.Fatalf("method = %q", method)
			}
			return nil, want
		},
	}
	_, err := m.Call(context.Background(), "getblockcount", nil)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}
