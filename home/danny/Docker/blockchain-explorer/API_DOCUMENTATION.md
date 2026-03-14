# Bitcoin Explorer API Documentation

## Overview

The Bitcoin Explorer provides a RESTful API for accessing blockchain data programmatically. This API allows developers to retrieve information about blocks, transactions, addresses, and network statistics.

## Base URL

```
http://localhost:8080/api
```

## Authentication

Most endpoints are publicly accessible. Admin endpoints require authentication via session tokens.

### Rate Limiting
- **Default**: 100 requests per minute per IP
- **Authenticated**: 500 requests per minute per user
- **Headers**: Rate limit information is included in response headers

## Response Format

All responses are in JSON format with the following structure:

### Success Response
```json
{
  "success": true,
  "data": { ... },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Error Response
```json
{
  "success": false,
  "error": "Error message",
  "code": "ERROR_CODE",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## API Endpoints

### Search API

#### Search Blockchain Data
Search for blocks, transactions, or addresses by various criteria.

**Endpoint:** `GET /api/search`

**Parameters:**
- `q` (required): Search query (block height, hash, transaction ID, or address)
- `type` (optional): Filter by type (`block`, `transaction`, `address`)
- `limit` (optional): Maximum results to return (default: 10, max: 100)

**Example Request:**
```bash
curl "http://localhost:8080/api/search?q=800000&type=block"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "results": [
      {
        "type": "block",
        "height": 800000,
        "hash": "0000000000000000000000000000000000000000000000000000000000000000",
        "timestamp": "2024-01-01T00:00:00Z",
        "transaction_count": 2500,
        "size": 1500000
      }
    ],
    "total": 1
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Autocomplete Suggestions
Get search suggestions as the user types.

**Endpoint:** `GET /api/autocomplete`

**Parameters:**
- `q` (required): Partial search query
- `limit` (optional): Maximum suggestions (default: 5, max: 20)

**Example Request:**
```bash
curl "http://localhost:8080/api/autocomplete?q=1A1zP"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "suggestions": [
      {
        "text": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
        "type": "address",
        "label": "Genesis Address"
      }
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Data Export & Reporting

Export endpoints return machine-friendly JSON suited for archival or analysis. Responses include `export_meta` (timestamp, version, endpoint), optional `pagination`, and `data`. Export routes respect the same authentication and pagination rules as their non-export counterparts.

#### Export Portfolios
Export the authenticated user's portfolios as JSON. **Requires authentication.**

**Endpoint:** `GET /api/user/portfolios/export`  
**Versioned:** `GET /api/v1/user/portfolios/export`

**Parameters:**
- `page` (optional): Page number (default: 1)
- `page_size` (optional): Items per page (default: 20, max: 100)
- `sort_by` (optional): Sort field (`created`, `updated`; default: `created`)
- `sort_dir` (optional): Sort direction (`asc`, `desc`; default: `desc`)

**Headers:**
- Session cookie or `Authorization` as required for user endpoints

**Example Request:**
```bash
curl -b cookies.txt "http://localhost:8080/api/user/portfolios/export?page=1&page_size=50"
```

**Example Response:**
```json
{
  "export_meta": {
    "export_timestamp": "2024-01-01T12:00:00Z",
    "export_version": "1.0",
    "endpoint": "portfolios"
  },
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 5,
    "total_pages": 1
  },
  "data": [
    {
      "id": "1234567890",
      "username": "user",
      "name": "My Portfolio",
      "description": "Holdings",
      "items": [],
      "created": "2024-01-01T00:00:00Z",
      "updated": "2024-01-01T12:00:00Z"
    }
  ]
}
```

#### Export Portfolio Holdings (CSV)
Stream a single portfolio's positions as CSV for download. **Requires authentication.** Response uses `Content-Type: text/csv` and `Content-Disposition: attachment` so browsers download a named file.

**Endpoint:** `GET /api/user/portfolios/:id/export/csv`  
**Versioned:** `GET /api/v1/user/portfolios/:id/export/csv`

**Parameters:**
- `id` (path): Portfolio ID

**Headers:**
- Session cookie or `Authorization` as required for user endpoints

**Response headers:**
- `Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename="portfolio-{id}-{name}.csv"`

**CSV columns:** `symbol`, `type`, `address`, `amount`, `value`, `portfolio_created`, `portfolio_updated`

**Example Request:**
```bash
curl -b cookies.txt -o holdings.csv "http://localhost:8080/api/user/portfolios/abc123/export/csv"
```

#### Export blocks (CSV)
Stream blocks in a height range as CSV. **Public.** Server-side streaming with one block fetched at a time; range and row limits prevent abuse.

**Endpoint:** `GET /api/blocks/export/csv`  
**Versioned:** `GET /api/v1/blocks/export/csv`

**Parameters:**
- `start_height` (required): First block height (inclusive)
- `end_height` (required): Last block height (inclusive). Must not exceed current chain height.
- `limit` (optional): Max rows to return (default: 500, max: 2000). Range size is also capped at 500 blocks.

**Response headers:**
- `Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename="blocks-{start}-{end}.csv"`

**CSV columns:** `height`, `hash`, `time`, `time_iso`, `tx_count`, `size`, `weight`, `difficulty`, `confirmations`

**Example Request:**
```bash
curl -o blocks.csv "http://localhost:8080/api/blocks/export/csv?start_height=800000&end_height=800050&limit=100"
```

#### Export transactions (CSV)
Stream transactions from blocks in a height range as CSV. **Public.** Memory-efficient: one block and one transaction at a time; block range and row limits prevent abuse.

**Endpoint:** `GET /api/transactions/export/csv`  
**Versioned:** `GET /api/v1/transactions/export/csv`

**Parameters:**
- `start_height` (required): First block height (inclusive)
- `end_height` (required): Last block height (inclusive). Must not exceed current chain height.
- `limit` (optional): Max transaction rows (default: 1000, max: 5000). Block range is capped at 100 blocks.

**Response headers:**
- `Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename="transactions-{start}-{end}.csv"`

**CSV columns:** `txid`, `block_height`, `block_hash`, `block_time`, `block_time_iso`, `size`, `vsize`, `weight`, `fee`, `locktime`, `version`

**Example Request:**
```bash
curl -o transactions.csv "http://localhost:8080/api/transactions/export/csv?start_height=800000&end_height=800010&limit=500"
```

#### Export Search (Blockchain)
Export a single blockchain search result (block, transaction, or address) as JSON. **Public;** no authentication required.

**Endpoint:** `GET /api/search/export`  
**Versioned:** `GET /api/v1/search/export`

**Parameters:**
- `q` (required): Search query (block height, hash, transaction ID, or address)

**Example Request:**
```bash
curl "http://localhost:8080/api/search/export?q=800000"
```

**Example Response:**
```json
{
  "export_meta": {
    "export_timestamp": "2024-01-01T12:00:00Z",
    "export_version": "1.0",
    "endpoint": "search",
    "query": "800000"
  },
  "data": {
    "type": "block",
    "result": {
      "height": 800000,
      "hash": "...",
      "timestamp": "2024-01-01T00:00:00Z"
    }
  }
}
```

#### Export Advanced Search (Symbols)
Export symbol search results with filters and sorting as JSON. **Public;** no authentication required. Supports pagination.

**Endpoint:** `GET /api/search/advanced/export`  
**Versioned:** `GET /api/v1/search/advanced/export`

**Parameters:**
- `q` (optional): Text search on symbol or name
- `types` (optional): Comma-separated types (e.g. `crypto`)
- `categories` (optional): Comma-separated categories (e.g. `defi,layer1`)
- `min_price`, `max_price` (optional): Price range
- `min_market_cap`, `max_market_cap` (optional): Market cap range
- `page` (optional): Page number (default: 1)
- `page_size` (optional): Items per page (default: 20, max: 100)
- `sort_by` (optional): Field to sort by (e.g. `rank`, `price`, `market_cap`)
- `sort_dir` (optional): `asc` or `desc` (default: `asc` for rank)

**Example Request:**
```bash
curl "http://localhost:8080/api/search/advanced/export?q=BTC&page=1&page_size=10"
```

**Example Response:**
```json
{
  "export_meta": {
    "export_timestamp": "2024-01-01T12:00:00Z",
    "export_version": "1.0",
    "endpoint": "search/advanced",
    "query": "BTC",
    "filters_applied": {},
    "sort_applied": { "field": "rank", "direction": "asc" }
  },
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total": 1,
    "total_pages": 1
  },
  "data": [
    {
      "symbol": "BTC",
      "name": "Bitcoin",
      "type": "crypto",
      "category": "layer1",
      "market_cap": 850000000000,
      "price": 45000,
      "volume_24h": 25000000000,
      "change_24h": 2.5,
      "rank": 1,
      "is_active": true,
      "listed_since": 1279408155
    }
  ]
}
```

### Block API

#### Get Block Details
Retrieve detailed information about a specific block.

**Endpoint:** `GET /api/block/{identifier}`

**Parameters:**
- `identifier` (path): Block height (number) or block hash (string)
- `include_txs` (optional): Include transaction details (`true`/`false`, default: `false`)

**Example Request:**
```bash
curl "http://localhost:8080/api/block/800000?include_txs=true"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "height": 800000,
    "hash": "0000000000000000000000000000000000000000000000000000000000000000",
    "previous_block_hash": "0000000000000000000000000000000000000000000000000000000000000001",
    "next_block_hash": "0000000000000000000000000000000000000000000000000000000000000002",
    "timestamp": "2024-01-01T00:00:00Z",
    "size": 1500000,
    "weight": 3996000,
    "transaction_count": 2500,
    "confirmations": 100,
    "difficulty": 50000000000000,
    "merkle_root": "0000000000000000000000000000000000000000000000000000000000000003",
    "nonce": 1234567890,
    "bits": "0x1a057a38",
    "version": 536870912,
    "miner": "F2Pool",
    "reward": 6.25,
    "transactions": [
      {
        "txid": "txid1",
        "size": 250,
        "fee": 0.0001
      }
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Get Latest Blocks
Retrieve the most recent blocks.

**Endpoint:** `GET /api/blocks/latest`

**Parameters:**
- `limit` (optional): Number of blocks to return (default: 10, max: 50)

**Example Request:**
```bash
curl "http://localhost:8080/api/blocks/latest?limit=5"
```

### Transaction API

#### Get Transaction Details
Retrieve detailed information about a specific transaction.

**Endpoint:** `GET /api/transaction/{txid}`

**Parameters:**
- `txid` (path): Transaction ID (hash)
- `include_raw` (optional): Include raw transaction data (`true`/`false`, default: `false`)

**Example Request:**
```bash
curl "http://localhost:8080/api/transaction/a1b2c3d4e5f6..."
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "txid": "a1b2c3d4e5f6...",
    "hash": "a1b2c3d4e5f6...",
    "version": 2,
    "size": 250,
    "vsize": 167,
    "weight": 668,
    "locktime": 0,
    "fee": 0.0001,
    "status": {
      "confirmed": true,
      "block_height": 800000,
      "block_hash": "0000000000000000000000000000000000000000000000000000000000000000",
      "block_time": "2024-01-01T00:00:00Z"
    },
    "inputs": [
      {
        "txid": "prev_txid",
        "vout": 0,
        "scriptsig": "script_data",
        "sequence": 4294967295,
        "witness": ["witness_data"],
        "prevout": {
          "scriptpubkey": "script_data",
          "scriptpubkey_asm": "asm_representation",
          "scriptpubkey_type": "p2wpkh",
          "scriptpubkey_address": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
          "value": 100000
        }
      }
    ],
    "outputs": [
      {
        "scriptpubkey": "script_data",
        "scriptpubkey_asm": "asm_representation",
        "scriptpubkey_type": "p2wpkh",
        "scriptpubkey_address": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
        "value": 50000
      }
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Get Latest Transactions
Retrieve the most recent transactions.

**Endpoint:** `GET /api/transactions/latest`

**Parameters:**
- `limit` (optional): Number of transactions to return (default: 10, max: 50)
- `min_value` (optional): Minimum transaction value in satoshis

**Example Request:**
```bash
curl "http://localhost:8080/api/transactions/latest?limit=10&min_value=100000000"
```

### Address API

#### Get Address Information
Retrieve information about a Bitcoin address.

**Endpoint:** `GET /api/address/{address}`

**Parameters:**
- `address` (path): Bitcoin address
- `include_txs` (optional): Include transaction history (`true`/`false`, default: `false`)
- `limit` (optional): Number of transactions to include (default: 10, max: 100)
- `offset` (optional): Offset for pagination (default: 0)

**Example Request:**
```bash
curl "http://localhost:8080/api/address/1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa?include_txs=true&limit=5"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "address": "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
    "chain_stats": {
      "funded_txo_count": 100,
      "funded_txo_sum": 500000000,
      "spent_txo_count": 90,
      "spent_txo_sum": 450000000,
      "tx_count": 50
    },
    "mempool_stats": {
      "funded_txo_count": 2,
      "funded_txo_sum": 10000000,
      "spent_txo_count": 1,
      "spent_txo_sum": 5000000,
      "tx_count": 3
    },
    "balance": 50000000,
    "total_received": 510000000,
    "total_sent": 460000000,
    "transactions": [
      {
        "txid": "txid1",
        "status": {
          "confirmed": true,
          "block_height": 800000,
          "block_hash": "0000000000000000000000000000000000000000000000000000000000000000",
          "block_time": "2024-01-01T00:00:00Z"
        },
        "value": 10000000,
        "fee": 1000
      }
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Network API

#### Network Status
Get current network statistics and status.

**Endpoint:** `GET /api/network-status`

**Example Request:**
```bash
curl "http://localhost:8080/api/network-status"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "network": "bitcoin",
    "best_block_height": 800000,
    "best_block_hash": "0000000000000000000000000000000000000000000000000000000000000000",
    "best_block_time": "2024-01-01T00:00:00Z",
    "difficulty": 50000000000000,
    "median_time": "2024-01-01T00:00:00Z",
    "chain": "main",
    "mempool_size": 5000,
    "mempool_total_fee": 0.5,
    "mempool_tps": 5.2,
    "hash_rate": 150000000,
    "next_retarget_time": "2024-01-08T00:00:00Z",
    "next_retarget_height": 800320,
    "estimated_retarget_ratio": 1.02
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Exchange Rates
Get current Bitcoin exchange rates.

**Endpoint:** `GET /api/rates`

**Parameters:**
- `currency` (optional): Target currency (default: `USD`)

**Example Request:**
```bash
curl "http://localhost:8080/api/rates?currency=EUR"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "USD": 50000.00,
    "EUR": 45000.00,
    "GBP": 40000.00,
    "JPY": 5500000.00,
    "timestamp": "2024-01-01T00:00:00Z"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

### Metrics API

#### Network Metrics
Get metrics data for charts and analytics.

**Endpoint:** `GET /api/metrics`

**Example Request:**
```bash
curl "http://localhost:8080/api/metrics"
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "mempool_size": [
      {"time": 1704067200, "value": 5000},
      {"time": 1704067500, "value": 5200},
      {"time": 1704067800, "value": 4800}
    ],
    "block_times": [
      {"time": 1704067200, "value": 600},
      {"time": 1704067500, "value": 580},
      {"time": 1704067800, "value": 620}
    ],
    "tx_volume": [
      {"time": 1704067200, "value": 150000000},
      {"time": 1704067500, "value": 175000000},
      {"time": 1704067800, "value": 160000000}
    ]
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## Admin API

### Authentication

#### Login
Authenticate as an admin user.

**Endpoint:** `POST /api/admin/login`

**Request Body:**
```json
{
  "username": "admin",
  "password": "admin123"
}
```

**Example Response:**
```json
{
  "success": true,
  "data": {
    "session_id": "session_token_here",
    "expires_at": "2024-01-02T00:00:00Z"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Logout
End the current admin session.

**Endpoint:** `POST /api/admin/logout`

**Headers:**
- `Authorization: Bearer {session_id}`

### Cache Management

#### Clear Cache
Clear the Redis cache.

**Endpoint:** `POST /api/admin/cache/clear`

**Headers:**
- `Authorization: Bearer {session_id}`

**Example Response:**
```json
{
  "success": true,
  "data": {
    "message": "Cache cleared successfully",
    "keys_cleared": 150
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

#### Cache Statistics
Get cache usage statistics.

**Endpoint:** `GET /api/admin/cache/stats`

**Headers:**
- `Authorization: Bearer {session_id}`

**Example Response:**
```json
{
  "success": true,
  "data": {
    "total_keys": 1000,
    "memory_usage": "50MB",
    "hit_rate": 0.85,
    "miss_rate": 0.15,
    "oldest_entry": "2024-01-01T00:00:00Z",
    "newest_entry": "2024-01-01T12:00:00Z"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## Error Codes

| Code | Description | HTTP Status |
|------|-------------|-------------|
| `INVALID_REQUEST` | Invalid request format | 400 |
| `NOT_FOUND` | Resource not found | 404 |
| `RATE_LIMITED` | Rate limit exceeded | 429 |
| `SERVER_ERROR` | Internal server error | 500 |
| `SERVICE_UNAVAILABLE` | Service temporarily unavailable | 503 |
| `UNAUTHORIZED` | Authentication required | 401 |
| `FORBIDDEN` | Access denied | 403 |

## Rate Limiting

Rate limits are enforced per IP address and per authenticated user:

- **Anonymous users**: 100 requests per minute
- **Authenticated users**: 500 requests per minute
- **Admin users**: 1000 requests per minute

Rate limit information is included in response headers:
- `X-RateLimit-Limit`: Maximum requests allowed
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: Timestamp when limit resets

## Caching

API responses are cached to improve performance:
- **Block data**: 5 minutes
- **Transaction data**: 5 minutes
- **Address data**: 2 minutes
- **Network status**: 30 seconds
- **Exchange rates**: 60 seconds

Use cache-busting parameters (`?t={timestamp}`) to force fresh data.

## WebSocket Support

Real-time updates are available via WebSocket for:
- New blocks
- New transactions
- Network status changes
- Price updates

**Connection URL:** `ws://localhost:8080/api/ws`

**Example Connection:**
```javascript
const ws = new WebSocket('ws://localhost:8080/api/ws');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('New update:', data);
};
```

## SDKs and Libraries

### JavaScript/Node.js
```javascript
const axios = require('axios');

const client = axios.create({
  baseURL: 'http://localhost:8080/api',
  timeout: 10000
});

// Search for a block
client.get('/search?q=800000&type=block')
  .then(response => console.log(response.data))
  .catch(error => console.error(error));
```

### Python
```python
import requests

BASE_URL = 'http://localhost:8080/api'

def search_block(height):
    response = requests.get(f'{BASE_URL}/search?q={height}&type=block')
    return response.json()

# Usage
result = search_block(800000)
print(result)
```

### Go
```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type SearchResult struct {
    Success bool `json:"success"`
    Data    struct {
        Results []interface{} `json:"results"`
    } `json:"data"`
}

func searchBlock(height int) (*SearchResult, error) {
    resp, err := http.Get(fmt.Sprintf("http://localhost:8080/api/search?q=%d&type=block", height))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result SearchResult
    err = json.NewDecoder(resp.Body).Decode(&result)
    return &result, err
}
```

## Best Practices

### Error Handling
- Always check the `success` field in responses
- Implement exponential backoff for retries
- Handle rate limiting gracefully
- Log errors for debugging

### Performance
- Use pagination for large result sets
- Cache responses appropriately
- Batch requests when possible
- Use WebSocket for real-time data

### Security
- Always use HTTPS in production
- Validate all inputs
- Implement proper authentication
- Monitor for suspicious activity

## Changelog

### Version 1.0.0 (Current)
- Initial API release
- Basic search functionality
- Block, transaction, and address endpoints
- Network status and metrics
- Admin authentication
- Rate limiting and caching

## Support

For API support:
- Check the troubleshooting section in the User Documentation
- Review server logs for error details
- Contact your system administrator
- Report bugs through appropriate channels