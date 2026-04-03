// Package pricing provides fiat currency rules, FX/crypto spot helpers, and pluggable price sources.
//
// Boundaries:
//   - fiat.go, fiat_convert.go — allowed fiat codes and conversion from multi-currency BTC quotes
//   - interfaces.go — contracts (Client, AssetPricer, asset-class fetchers); mock implementations in mock.go
//   - coingecko.go — HTTP calls to CoinGecko only (no caching); BaseURL on CoinGeckoClient for tests
//   - coingecko_contract_test.go — fixture replay + strict timeouts (no live network)
//   - cache.go — optional in-memory TTL cache around Client and CryptoPriceFetcher
//   - assets.go — CompositePricer and symbol normalization (orchestrates fetchers)
//   - commodity.go, bond.go — static non-crypto sources
package pricing
