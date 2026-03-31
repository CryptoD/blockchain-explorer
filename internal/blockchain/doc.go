// Package blockchain defines the Blockchain JSON-RPC interface for GetBlock-compatible Bitcoin nodes,
// a concrete HTTP client (GetBlockRPCClient), and test doubles. Call sites should use the interface;
// metrics and request deadlines are applied at the application boundary (internal/server callBlockchain).
package blockchain
