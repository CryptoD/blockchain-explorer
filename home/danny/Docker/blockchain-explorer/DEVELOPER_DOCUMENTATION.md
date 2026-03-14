# Bitcoin Explorer - Developer Documentation

## Table of Contents
1. [Architecture Overview](#architecture-overview)
2. [Project Structure](#project-structure)
3. [Technology Stack](#technology-stack)
4. [Development Environment Setup](#development-environment-setup)
5. [Backend Development](#backend-development)
6. [Frontend Development](#frontend-development)
7. [Database Design](#database-design)
8. [Caching Strategy](#caching-strategy)
9. [Testing](#testing)
10. [Deployment](#deployment)
11. [Monitoring and Logging](#monitoring-and-logging)
12. [Security Considerations](#security-considerations)
13. [Performance Optimization](#performance-optimization)
14. [Contributing Guidelines](#contributing-guidelines)

## Architecture Overview

The Bitcoin Explorer follows a microservices-inspired architecture with clear separation of concerns:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │────▶│   Backend API   │────▶│  Blockchain     │
│  (HTML/JS/CSS) │     │   (Go/Gin)      │     │  Node/API       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
       │                        │                        │
       ▼                        ▼                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   CDN/Static    │     │   Redis Cache   │     │   PostgreSQL    │
│   Assets        │     │   (Caching)     │     │   (Persistence) │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Key Components
- **Frontend**: Single-page application with responsive design
- **Backend API**: RESTful API built with Go and Gin framework
- **Cache Layer**: Redis for performance optimization
- **Database**: PostgreSQL for persistent data storage
- **Blockchain Integration**: Direct RPC connection to Bitcoin node

## Project Structure

```
bitcoin-explorer/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Go dependencies
├── bitcoin.html            # Main frontend file
├── index.html              # Alternative frontend
├── script.js               # Frontend JavaScript
├── styles.css              # Frontend styles
├── dist/                   # Built frontend assets
│   └── styles.css           # Compiled Tailwind CSS
├── src/                    # Source files
│   └── styles.css           # Tailwind source
├── images/                 # Static images and assets
├── k8s/                    # Kubernetes deployment files
├── terraform/              # Infrastructure as code
├── docker-compose.yml      # Local development setup
├── Dockerfile              # Container definition
├── package.json            # Node.js dependencies
├── tailwind.config.js      # Tailwind CSS configuration
├── postcss.config.js       # PostCSS configuration
└── readme.md               # Project documentation
```

### Key Files Explained
- **main.go**: Core application logic, API endpoints, and business logic
- **bitcoin.html**: Main user interface with search, charts, and data display
- **script.js**: Client-side JavaScript for interactivity and API calls
- **styles.css**: Custom styles and Tailwind CSS utilities
- **docker-compose.yml**: Local development environment setup

## Technology Stack

### Backend
- **Language**: Go 1.21+
- **Framework**: Gin (HTTP web framework)
- **Caching**: Redis with go-redis client
- **HTTP Client**: go-resty for API calls
- **Logging**: sirupsen/logrus
- **Error Tracking**: Sentry integration
- **Configuration**: Environment variables

### Frontend
- **HTML5**: Semantic markup with accessibility features
- **CSS3**: Tailwind CSS for utility-first styling
- **JavaScript**: Vanilla JS for interactivity
- **Charts**: Chart.js for data visualization
- **Internationalization**: i18next for multi-language support

### Infrastructure
- **Containerization**: Docker and Docker Compose
- **Orchestration**: Kubernetes (optional)
- **Reverse Proxy**: Nginx (recommended for production)
- **SSL/TLS**: Let's Encrypt integration
- **Monitoring**: Prometheus and Grafana (optional)

### Development Tools
- **Build Tools**: PostCSS, Autoprefixer
- **Testing**: Go testing framework
- **Linting**: golangci-lint
- **Formatting**: gofmt, goimports

## Development Environment Setup

### Prerequisites
- Go 1.21 or higher
- Node.js 16+ and npm
- Docker and Docker Compose
- Redis server
- Bitcoin node (optional, for full functionality)

### Local Development Setup

1. **Clone the repository:**
```bash
git clone <repository-url>
cd bitcoin-explorer
```

2. **Install Go dependencies:**
```bash
go mod tidy
go mod download
```

3. **Install Node.js dependencies:**
```bash
npm install
```

4. **Build frontend assets:**
```bash
npm run build
```

5. **Set up environment variables:**
```bash
cp .env.example .env
# Edit .env with your configuration
```

6. **Start Redis:**
```bash
redis-server
```

7. **Run the application:**
```bash
go run main.go
```

### Docker Development Setup

1. **Start all services:**
```bash
docker-compose up --build
```

2. **Access the application:**
- Frontend: http://localhost:8080
- Adminer (database admin): http://localhost:8080
- Redis: localhost:6379

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `8080` | No |
| `REDIS_HOST` | Redis server host | `localhost` | No |
| `REDIS_PORT` | Redis server port | `6379` | No |
| `REDIS_PASSWORD` | Redis password | `` | No |
| `SENTRY_DSN` | Sentry error tracking DSN | `` | No |
| `ADMIN_USERNAME` | Admin username | `admin` | No |
| `ADMIN_PASSWORD` | Admin password | `admin123` | No |
| `GETBLOCK_API_KEY` | GetBlock API key | `` | Yes |
| `GETBLOCK_BASE_URL` | GetBlock API base URL | `` | Yes |
| `RATE_LIMIT_PER_MINUTE` | Rate limit per IP | `100` | No |
| `CACHE_TTL_BLOCKS` | Block cache TTL | `5m` | No |
| `CACHE_TTL_TXS` | Transaction cache TTL | `5m` | No |
| `CACHE_TTL_ADDRESS` | Address cache TTL | `2m` | No |

## Backend Development

### API Endpoint Structure

All API endpoints follow RESTful conventions:

```go
// Example endpoint structure
r.GET("/api/resource", handler)
r.GET("/api/resource/:id", handler)
r.POST("/api/resource", handler)
r.PUT("/api/resource/:id", handler)
r.DELETE("/api/resource/:id", handler)
```

### Handler Function Pattern

```go
func exampleHandler(c *gin.Context) {
    // 1. Parse request parameters
    id := c.Param("id")
    limit := c.DefaultQuery("limit", "10")
    
    // 2. Validate inputs
    if id == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID is required"})
        return
    }
    
    // 3. Check cache first
    cacheKey := fmt.Sprintf("resource:%s", id)
    if cached, err := getFromCache(cacheKey); err == nil {
        c.JSON(http.StatusOK, gin.H{"data": cached})
        return
    }
    
    // 4. Fetch from external API or database
    data, err := fetchResource(id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resource"})
        return
    }
    
    // 5. Cache the result
    setCache(cacheKey, data, 5*time.Minute)
    
    // 6. Return response
    c.JSON(http.StatusOK, gin.H{"data": data})
}
```

### Error Handling

Custom error types for better error handling:

```go
type APIError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Status  int    `json:"status"`
}

func (e *APIError) Error() string {
    return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Usage
func handleError(c *gin.Context, err error) {
    if apiErr, ok := err.(*APIError); ok {
        c.JSON(apiErr.Status, gin.H{
            "error": apiErr.Message,
            "code": apiErr.Code,
        })
        return
    }
    
    // Default error handling
    c.JSON(http.StatusInternalServerError, gin.H{
        "error": "Internal server error",
    })
}
```

### Logging Standards

Structured logging using logrus:

```go
import log "github.com/sirupsen/logrus"

func someFunction() {
    log.WithFields(log.Fields{
        "user_id": userID,
        "action": "search",
        "query": query,
    }).Info("User performed search")
    
    log.WithError(err).WithFields(log.Fields{
        "operation": "fetch_block",
        "block_height": height,
    }).Error("Failed to fetch block data")
}
```

### Testing Backend Code

Unit test example:

```go
func TestSearchHandler(t *testing.T) {
    // Setup
    gin.SetMode(gin.TestMode)
    router := gin.New()
    router.GET("/api/search", searchHandler)
    
    // Test cases
    tests := []struct {
        name           string
        query          string
        expectedStatus int
        expectedBody   string
    }{
        {
            name:           "Valid block search",
            query:          "800000",
            expectedStatus: http.StatusOK,
            expectedBody:   "success",
        },
        {
            name:           "Invalid search query",
            query:          "",
            expectedStatus: http.StatusBadRequest,
            expectedBody:   "error",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req, _ := http.NewRequest("GET", "/api/search?q="+tt.query, nil)
            w := httptest.NewRecorder()
            router.ServeHTTP(w, req)
            
            assert.Equal(t, tt.expectedStatus, w.Code)
            assert.Contains(t, w.Body.String(), tt.expectedBody)
        })
    }
}
```

## Frontend Development

### HTML Structure

Semantic HTML with accessibility:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Bitcoin Explorer</title>
    <meta name="description" content="Bitcoin blockchain explorer">
    <link rel="canonical" href="/">
</head>
<body>
    <header role="banner">
        <nav role="navigation" aria-label="Main navigation">
            <!-- Navigation content -->
        </nav>
        <form role="search" aria-label="Blockchain search">
            <label for="search-input" class="sr-only">Search blockchain</label>
            <input id="search-input" type="search" placeholder="Search...">
            <button type="submit">Search</button>
        </form>
    </header>
    
    <main role="main">
        <section aria-label="Latest blocks">
            <h2>Latest Blocks</h2>
            <!-- Block content -->
        </section>
        
        <section aria-label="Latest transactions">
            <h2>Latest Transactions</h2>
            <!-- Transaction content -->
        </section>
    </main>
    
    <footer role="contentinfo">
        <!-- Footer content -->
    </footer>
</body>
</html>
```

### JavaScript Architecture

Modular JavaScript with async/await:

```javascript
// API client module
const APIClient = {
    baseURL: '/api',
    
    async search(query, type = null) {
        const params = new URLSearchParams({ q: query });
        if (type) params.append('type', type);
        
        try {
            const response = await fetch(`${this.baseURL}/search?${params}`);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            return await response.json();
        } catch (error) {
            console.error('Search failed:', error);
            throw error;
        }
    },
    
    async getBlock(identifier) {
        const response = await fetch(`${this.baseURL}/block/${identifier}`);
        return response.json();
    },
    
    async getTransaction(txid) {
        const response = await fetch(`${this.baseURL}/transaction/${txid}`);
        return response.json();
    }
};

// UI module
const UI = {
    showLoading(element) {
        element.innerHTML = '<div class="loading" aria-label="Loading">Loading...</div>';
    },
    
    showError(element, message) {
        element.innerHTML = `<div class="error" role="alert">Error: ${message}</div>`;
    },
    
    formatNumber(num) {
        return new Intl.NumberFormat().format(num);
    },
    
    formatBTC(satoshis) {
        return (satoshis / 100000000).toFixed(8);
    }
};
```

### CSS Architecture

Utility-first CSS with Tailwind:

```css
/* Custom utilities */
@layer utilities {
    .text-balance {
        @apply text-gray-900 font-mono;
    }
    
    .loading {
        @apply animate-pulse bg-gray-200 rounded;
    }
    
    .error {
        @apply text-red-600 bg-red-50 border border-red-200 rounded p-4;
    }
}

/* Component styles */
@layer components {
    .search-input {
        @apply w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent;
    }
    
    .btn-primary {
        @apply bg-blue-600 text-white px-4 py-2 rounded-lg hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500;
    }
    
    .card {
        @apply bg-white rounded-lg shadow-md p-6 border border-gray-200;
    }
}
```

### Chart Integration

Chart.js integration for data visualization:

```javascript
function renderChart(data, elementId, chartType = 'line') {
    const ctx = document.getElementById(elementId).getContext('2d');
    
    const config = {
        type: chartType,
        data: {
            labels: data.map(d => new Date(d.time * 1000).toLocaleDateString()),
            datasets: [{
                label: 'Data',
                data: data.map(d => d.value),
                borderColor: 'rgb(75, 192, 192)',
                backgroundColor: 'rgba(75, 192, 192, 0.2)',
                tension: 0.1
            }]
        },
        options: {
            responsive: true,
            scales: {
                y: {
                    beginAtZero: true
                }
            },
            plugins: {
                legend: {
                    display: true
                },
                tooltip: {
                    mode: 'index',
                    intersect: false
                }
            }
        }
    };
    
    new Chart(ctx, config);
}
```

### Internationalization

Multi-language support with i18next:

```javascript
// Initialize i18next
i18next.init({
    lng: 'en',
    resources: {
        en: {
            translation: {
                "search_placeholder": "Search by block height, transaction hash, or address...",
                "latest_blocks": "Latest Blocks",
                "latest_transactions": "Latest Transactions",
                "block_height": "Block Height",
                "transaction_id": "Transaction ID"
            }
        },
        es: {
            translation: {
                "search_placeholder": "Buscar por altura de bloque, hash de transacción o dirección...",
                "latest_blocks": "Últimos Bloques",
                "latest_transactions": "Últimas Transacciones",
                "block_height": "Altura del Bloque",
                "transaction_id": "ID de Transacción"
            }
        }
    }
}, function(err, t) {
    // Update content
    updateContent();
});

function updateContent() {
    document.querySelectorAll('[data-i18n]').forEach(element => {
        const key = element.getAttribute('data-i18n');
        element.textContent = i18next.t(key);
    });
}
```

## Database Design

### PostgreSQL Schema

```sql
-- Blocks table
CREATE TABLE blocks (
    id SERIAL PRIMARY KEY,
    height INTEGER UNIQUE NOT NULL,
    hash VARCHAR(64) UNIQUE NOT NULL,
    previous_block_hash VARCHAR(64),
    timestamp TIMESTAMP NOT NULL,
    size INTEGER NOT NULL,
    weight INTEGER NOT NULL,
    transaction_count INTEGER NOT NULL,
    difficulty NUMERIC NOT NULL,
    merkle_root VARCHAR(64) NOT NULL,
    nonce BIGINT NOT NULL,
    bits VARCHAR(10) NOT NULL,
    version INTEGER NOT NULL,
    miner VARCHAR(100),
    reward NUMERIC(16, 8) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Transactions table
CREATE TABLE transactions (
    id SERIAL PRIMARY KEY,
    txid VARCHAR(64) UNIQUE NOT NULL,
    hash VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    size INTEGER NOT NULL,
    vsize INTEGER NOT NULL,
    weight INTEGER NOT NULL,
    locktime INTEGER NOT NULL,
    fee NUMERIC(16, 8) NOT NULL,
    block_height INTEGER REFERENCES blocks(height),
    block_hash VARCHAR(64),
    block_time TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Addresses table
CREATE TABLE addresses (
    id SERIAL PRIMARY KEY,
    address VARCHAR(100) UNIQUE NOT NULL,
    balance NUMERIC(16, 8) DEFAULT 0,
    total_received NUMERIC(16, 8) DEFAULT 0,
    total_sent NUMERIC(16, 8) DEFAULT 0,
    transaction_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for performance
CREATE INDEX idx_blocks_height ON blocks(height);
CREATE INDEX idx_blocks_hash ON blocks(hash);
CREATE INDEX idx_blocks_timestamp ON blocks(timestamp);
CREATE INDEX idx_transactions_txid ON transactions(txid);
CREATE INDEX idx_transactions_block_height ON transactions(block_height);
CREATE INDEX idx_addresses_address ON addresses(address);
```

### Redis Cache Structure

```go
// Cache key patterns
const (
    CacheKeyBlock         = "block:%s"           // block hash or height
    CacheKeyTransaction   = "tx:%s"              // transaction ID
    CacheKeyAddress       = "address:%s"         // address
    CacheKeyLatestBlocks  = "latest_blocks"      // latest blocks list
    CacheKeyLatestTxs     = "latest_transactions" // latest transactions
    CacheKeyNetworkStatus = "network_status"      // network statistics
    CacheKeyMetrics       = "metrics"             // chart data
    CacheKeyRates         = "rates"               // exchange rates
)

// Cache TTL values
const (
    CacheTTLBlock        = 5 * time.Minute
    CacheTTLTransaction  = 5 * time.Minute
    CacheTTLAddress      = 2 * time.Minute
    CacheTTLNetwork      = 30 * time.Second
    CacheTTLRates        = 60 * time.Second
    CacheTTLMetrics      = 5 * time.Minute
)
```

## Caching Strategy

### Multi-Level Caching

1. **Browser Cache**: Static assets (CSS, JS, images)
2. **CDN Cache**: Global distribution of static content
3. **Application Cache**: Redis for API responses
4. **Database Cache**: Query result caching

### Cache Invalidation

```go
// Cache invalidation on new block
func invalidateBlockCache(blockHash string) {
    // Remove specific block cache
    rdb.Del(ctx, fmt.Sprintf(CacheKeyBlock, blockHash))
    
    // Remove latest blocks cache
    rdb.Del(ctx, CacheKeyLatestBlocks)
    
    // Remove network status cache
    rdb.Del(ctx, CacheKeyNetworkStatus)
    
    // Update metrics cache
    updateMetricsCache()
}

// Background cache refresh
func startCacheRefresh() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for {
        refreshStaleCache()
        <-ticker.C
    }
}
```

### Cache Warming

```go
// Pre-populate cache with frequently accessed data
func warmCache() {
    // Cache latest blocks
    blocks, _ := fetchLatestBlocks(10)
    blocksJSON, _ := json.Marshal(blocks)
    rdb.Set(ctx, CacheKeyLatestBlocks, blocksJSON, CacheTTLBlock)
    
    // Cache network status
    status, _ := getNetworkStatus()
    statusJSON, _ := json.Marshal(status)
    rdb.Set(ctx, CacheKeyNetworkStatus, statusJSON, CacheTTLNetwork)
    
    // Cache exchange rates
    rates, _ := getExchangeRates()
    ratesJSON, _ := json.Marshal(rates)
    rdb.Set(ctx, CacheKeyRates, ratesJSON, CacheTTLRates)
}
```

## Testing

### Unit Testing

```go
func TestSearchHandler(t *testing.T) {
    // Setup test environment
    setupTestEnv()
    
    // Create test cases
    tests := []struct {
        name           string
        query          string
        expectedStatus int
        expectedResult bool
    }{
        {
            name:           "Valid block search",
            query:          "800000",
            expectedStatus: http.StatusOK,
            expectedResult: true,
        },
        {
            name:           "Invalid search query",
            query:          "invalid_query_that_is_too_long_and_should_fail",
            expectedStatus: http.StatusBadRequest,
            expectedResult: false,
        },
    }
    
    // Run tests
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            req := httptest.NewRequest("GET", "/api/search?q="+tt.query, nil)
            w := httptest.NewRecorder()
            
            searchHandler(w, req)
            
            assert.Equal(t, tt.expectedStatus, w.Code)
            
            var response map[string]interface{}
            json.Unmarshal(w.Body.Bytes(), &response)
            assert.Equal(t, tt.expectedResult, response["success"])
        })
    }
}
```

### Integration Testing

```go
func TestAPIIntegration(t *testing.T) {
    // Start test server
    server := setupTestServer()
    defer server.Close()
    
    // Test API endpoints
    client := &http.Client{}
    
    // Test search endpoint
    resp, err := client.Get(server.URL + "/api/search?q=800000")
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // Test block endpoint
    resp, err = client.Get(server.URL + "/api/block/800000")
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // Test transaction endpoint
    resp, err = client.Get(server.URL + "/api/transaction/some_txid")
    assert.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
}
```

### Frontend Testing

```javascript
// Jest test for JavaScript functions
import { formatBTC, formatNumber } from './utils';

describe('Utility Functions', () => {
    test('formatBTC converts satoshis to BTC', () => {
        expect(formatBTC(100000000)).toBe('1.00000000');
        expect(formatBTC(50000000)).toBe('0.50000000');
    });
    
    test('formatNumber adds thousand separators', () => {
        expect(formatNumber(1000)).toBe('1,000');
        expect(formatNumber(1000000)).toBe('1,000,000');
    });
});

// Integration test for API calls
describe('API Client', () => {
    test('search function calls correct endpoint', async () => {
        global.fetch = jest.fn(() =>
            Promise.resolve({
                ok: true,
                json: () => Promise.resolve({ success: true, data: {} }),
            })
        );
        
        const result = await APIClient.search('800000');
        expect(global.fetch).toHaveBeenCalledWith('/api/search?q=800000');
        expect(result.success).toBe(true);
    });
});
```

### Load Testing

```bash
# Using Apache Bench for load testing
ab -n 1000 -c 10 http://localhost:8080/api/search?q=800000

# Using wrk for more advanced load testing
wrk -t12 -c400 -d30s http://localhost:8080/api/search?q=800000

# Using k6 for scripted load testing
k6 run load-test.js
```

## Deployment

### Docker Deployment

```dockerfile
# Multi-stage build for smaller image
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o bitcoin-explorer .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/bitcoin-explorer .
COPY --from=builder /app/bitcoin.html .
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/images ./images

EXPOSE 8080
CMD ["./bitcoin-explorer"]
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bitcoin-explorer
spec:
  replicas: 3
  selector:
    matchLabels:
      app: bitcoin-explorer
  template:
    metadata:
      labels:
        app: bitcoin-explorer
    spec:
      containers:
      - name: bitcoin-explorer
        image: bitcoin-explorer:latest
        ports:
        - containerPort: 8080
        env:
        - name: REDIS_HOST
          value: "redis-service"
        - name: GETBLOCK_API_KEY
          valueFrom:
            secretKeyRef:
              name: bitcoin-explorer-secrets
              key: api-key
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: bitcoin-explorer-service
spec:
  selector:
    app: bitcoin-explorer
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
  type: LoadBalancer
```

### Production Checklist

- [ ] Environment variables configured
- [ ] SSL/TLS certificates installed
- [ ] Rate limiting configured
- [ ] Monitoring and logging setup
- [ ] Backup strategy implemented
- [ ] Security headers configured
- [ ] CORS policy configured
- [ ] Health check endpoints working
- [ ] Auto-scaling configured
- [ ] Database backups scheduled

## Monitoring and Logging

### Application Metrics

```go
// Prometheus metrics
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )
    
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "http_request_duration_seconds",
            Help: "HTTP request duration in seconds",
        },
        []string{"method", "endpoint"},
    )
    
    cacheHits = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "cache_hits_total",
            Help: "Total number of cache hits",
        },
    )
    
    cacheMisses = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "cache_misses_total",
            Help: "Total number of cache misses",
        },
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal)
    prometheus.MustRegister(httpRequestDuration)
    prometheus.MustRegister(cacheHits)
    prometheus.MustRegister(cacheMisses)
}
```

### Structured Logging

```go
// Structured logging with context
func logRequest(c *gin.Context) {
    start := time.Now()
    
    // Log request
    log.WithFields(log.Fields{
        "method":     c.Request.Method,
        "path":       c.Request.URL.Path,
        "ip":         c.ClientIP(),
        "user_agent": c.Request.UserAgent(),
    }).Info("HTTP request started")
    
    // Continue processing
    c.Next()
    
    // Log response
    duration := time.Since(start)
    log.WithFields(log.Fields{
        "method":     c.Request.Method,
        "path":       c.Request.URL.Path,
        "status":     c.Writer.Status(),
        "duration":   duration.Milliseconds(),
        "ip":         c.ClientIP(),
    }).Info("HTTP request completed")
    
    // Update metrics
    httpRequestsTotal.WithLabelValues(
        c.Request.Method,
        c.Request.URL.Path,
        strconv.Itoa(c.Writer.Status()),
    ).Inc()
    
    httpRequestDuration.WithLabelValues(
        c.Request.Method,
        c.Request.URL.Path,
    ).Observe(duration.Seconds())
}
```

### Health Checks

```go
// Health check endpoint
func healthCheckHandler(c *gin.Context) {
    health := map[string]interface{}{
        "status": "healthy",
        "timestamp": time.Now().UTC(),
        "version": version,
    }
    
    // Check Redis connection
    if err := rdb.Ping(ctx).Err(); err != nil {
        health["redis"] = "unhealthy"
        health["status"] = "unhealthy"
    } else {
        health["redis"] = "healthy"
    }
    
    // Check API connectivity
    if err := checkAPIConnectivity(); err != nil {
        health["api"] = "unhealthy"
        health["status"] = "unhealthy"
    } else {
        health["api"] = "healthy"
    }
    
    status := http.StatusOK
    if health["status"] == "unhealthy" {
        status = http.StatusServiceUnavailable
    }
    
    c.JSON(status, health)
}
```

## Security Considerations

### Input Validation

```go
// Input validation
func validateSearchQuery(query string) error {
    // Length validation
    if len(query) < 1 || len(query) > 100 {
        return errors.New("query must be between 1 and 100 characters")
    }
    
    // Character validation
    if matched, _ := regexp.MatchString("^[a-zA-Z0-9]+$", query); !matched {
        return errors.New("query contains invalid characters")
    }
    
    // Bitcoin address validation
    if isValidAddress(query) {
        return nil
    }
    
    // Transaction ID validation
    if isValidTransactionID(query) {
        return nil
    }
    
    // Block height validation
    if _, err := strconv.Atoi(query); err == nil {
        return nil
    }
    
    return errors.New("invalid search query format")
}
```

### Rate Limiting

```go
// Rate limiting middleware
func rateLimitMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        clientIP := c.ClientIP()
        
        // Check rate limit
        if isRateLimited(clientIP) {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
                "retry_after": getRetryAfter(clientIP),
            })
            c.Abort()
            return
        }
        
        c.Next()
    }
}

func isRateLimited(ip string) bool {
    key := fmt.Sprintf("rate_limit:%s", ip)
    
    // Get current count
    count, err := rdb.Get(ctx, key).Int()
    if err == redis.Nil {
        count = 0
    }
    
    // Check if limit exceeded
    if count >= rateLimitPerMinute {
        return true
    }
    
    // Increment counter
    pipe := rdb.Pipeline()
    pipe.Incr(ctx, key)
    pipe.Expire(ctx, key, time.Minute)
    pipe.Exec(ctx)
    
    return false
}
```

### CORS Configuration

```go
// CORS middleware
func corsMiddleware() gin.HandlerFunc {
    return cors.New(cors.Config{
        AllowOrigins:     []string{"https://explorer.example.com"},
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
        ExposeHeaders:    []string{"Content-Length", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
        AllowCredentials: true,
        MaxAge:           12 * time.Hour,
    })
}
```

### Security Headers

```go
// Security headers middleware
func securityHeaders() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' cdn.jsdelivr.net; style-src 'self' 'unsafe-inline'")
        c.Next()
    }
}
```

## Performance Optimization

### Database Optimization

```sql
-- Create indexes for common queries
CREATE INDEX CONCURRENTLY idx_blocks_height_timestamp ON blocks(height, timestamp);
CREATE INDEX CONCURRENTLY idx_transactions_block_height ON transactions(block_height);
CREATE INDEX CONCURRENTLY idx_addresses_balance ON addresses(balance DESC);

-- Partition large tables
CREATE TABLE transactions_2024 PARTITION OF transactions
    FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');

-- Use materialized views for complex queries
CREATE MATERIALIZED VIEW daily_metrics AS
SELECT 
    DATE(timestamp) as date,
    COUNT(*) as block_count,
    AVG(transaction_count) as avg_tx_per_block,
    SUM(size) as total_size
FROM blocks
GROUP BY DATE(timestamp);
```

### Query Optimization

```go
// Use prepared statements
const searchQuery = `
    SELECT * FROM blocks 
    WHERE height = $1 OR hash = $2 
    ORDER BY timestamp DESC 
    LIMIT 1
`

// Batch operations
func batchGetTransactions(txids []string) ([]Transaction, error) {
    placeholders := make([]string, len(txids))
    args := make([]interface{}, len(txids))
    
    for i, txid := range txids {
        placeholders[i] = fmt.Sprintf("$%d", i+1)
        args[i] = txid
    }
    
    query := fmt.Sprintf(`
        SELECT * FROM transactions 
        WHERE txid IN (%s)
    `, strings.Join(placeholders, ","))
    
    return db.Query(query, args...)
}
```

### Frontend Optimization

```javascript
// Lazy loading for charts
const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            loadChart(entry.target);
            observer.unobserve(entry.target);
        }
    });
});

document.querySelectorAll('.chart-container').forEach(chart => {
    observer.observe(chart);
});

// Debounce search input
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

const debouncedSearch = debounce((query) => {
    performSearch(query);
}, 300);

document.getElementById('search-input').addEventListener('input', (e) => {
    debouncedSearch(e.target.value);
});
```

## Contributing Guidelines

### Code Style

- Follow Go best practices and idioms
- Use meaningful variable and function names
- Write comprehensive comments for complex logic
- Keep functions small and focused
- Use consistent error handling patterns

### Git Workflow

1. Create feature branch from main
2. Make changes and commit with descriptive messages
3. Write/update tests for new functionality
4. Ensure all tests pass
5. Create pull request with detailed description
6. Address review feedback
7. Merge after approval

### Commit Message Format

```
type(scope): subject

body

footer
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes
- `refactor`: Code refactoring
- `test`: Test additions or changes
- `chore`: Build process or auxiliary tool changes

Example:
```
feat(api): add transaction history endpoint

- Add pagination support for large transaction lists
- Include fee information in response
- Add comprehensive tests

Closes #123
```

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added/updated
```

## Resources and References

### Documentation
- [Go Documentation](https://golang.org/doc/)
- [Gin Framework](https://github.com/gin-gonic/gin)
- [Redis Commands](https://redis.io/commands)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Chart.js Documentation](https://www.chartjs.org/docs/)

### Bitcoin Resources
- [Bitcoin Developer Guide](https://bitcoin.org/en/developer-guide)
- [Bitcoin RPC API Reference](https://bitcoin.org/en/developer-reference#rpcs)
- [Blockstream API](https://blockstream.info/api/)

### Best Practices
- [Go Best Practices](https://golang.org/doc/effective_go.html)
- [REST API Design](https://restfulapi.net/)
- [Web Security Best Practices](https://owasp.org/www-project-top-ten/)

### Tools and Utilities
- [Postman](https://www.postman.com/) - API testing
- [Redis CLI](https://redis.io/topics/rediscli) - Redis management
- [Docker](https://www.docker.com/) - Containerization
- [Kubernetes](https://kubernetes.io/) - Orchestration

---

For additional support and questions, please refer to the project repository or contact the development team.