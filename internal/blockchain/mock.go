package blockchain

import (
	"context"
	"errors"

	"github.com/go-resty/resty/v2"
)

// MockRPCClient implements Blockchain for tests. Set CallFunc to control responses
// without dialing real GetBlock or other JSON-RPC endpoints.
type MockRPCClient struct {
	CallFunc func(ctx context.Context, method string, params []interface{}) (*resty.Response, error)
}

// Call implements Blockchain.
func (m *MockRPCClient) Call(ctx context.Context, method string, params []interface{}) (*resty.Response, error) {
	if m == nil || m.CallFunc == nil {
		return nil, errors.New("blockchain.MockRPCClient: CallFunc not set")
	}
	return m.CallFunc(ctx, method, params)
}

var _ Blockchain = (*MockRPCClient)(nil)
