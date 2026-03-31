package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// Blockchain is the JSON-RPC surface for a GetBlock-compatible Bitcoin node.
// All explorer RPC traffic should go through this interface (see server callBlockchain).
type Blockchain interface {
	Call(ctx context.Context, method string, params []interface{}) (*resty.Response, error)
}

// RPCClient is an alias for Blockchain.
type RPCClient = Blockchain

// GetBlockRPCClient is a concrete implementation of Blockchain that talks to
// a GetBlock-compatible JSON-RPC endpoint.
type GetBlockRPCClient struct {
	BaseURL    string
	APIKey     string
	HTTPClient *resty.Client
}

// NewGetBlockRPCClient constructs a GetBlockRPCClient using the provided base
// URL, API key, and HTTP client. HTTPClient must be non-nil.
func NewGetBlockRPCClient(baseURL, apiKey string, httpClient *resty.Client) *GetBlockRPCClient {
	if httpClient == nil {
		httpClient = resty.New().
			SetTimeout(10 * time.Second).
			SetRetryCount(3)
	}
	return &GetBlockRPCClient{
		BaseURL:    baseURL,
		APIKey:     apiKey,
		HTTPClient: httpClient,
	}
}

// Call performs a single JSON-RPC call to the configured GetBlock endpoint.
func (c *GetBlockRPCClient) Call(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
	if c.BaseURL == "" || c.APIKey == "" {
		return nil, fmt.Errorf("blockchain RPC client not configured: missing base URL or API key")
	}

	requestID := fmt.Sprintf("%d", time.Now().UnixNano())
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	}

	resp, err := c.HTTPClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("x-api-key", c.APIKey).
		SetBody(payload).
		Post(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("blockchain RPC request failed: %w", err)
	}
	if resp.StatusCode() >= 400 {
		return nil, fmt.Errorf("blockchain RPC error: %s", resp.Status())
	}
	return resp, nil
}

var _ Blockchain = (*GetBlockRPCClient)(nil)
