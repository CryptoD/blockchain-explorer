# Bitcoin Blockchain Explorer

[![CI](https://github.com/CryptoD/blockchain-explorer/actions/workflows/ci.yml/badge.svg)](https://github.com/CryptoD/blockchain-explorer/actions/workflows/ci.yml)
[![Race](https://github.com/CryptoD/blockchain-explorer/actions/workflows/race.yml/badge.svg)](https://github.com/CryptoD/blockchain-explorer/actions/workflows/race.yml)
[![E2E](https://github.com/CryptoD/blockchain-explorer/actions/workflows/e2e.yml/badge.svg)](https://github.com/CryptoD/blockchain-explorer/actions/workflows/e2e.yml)
[![Mutation](https://github.com/CryptoD/blockchain-explorer/actions/workflows/mutation.yml/badge.svg)](https://github.com/CryptoD/blockchain-explorer/actions/workflows/mutation.yml)
[![Gitleaks](https://github.com/CryptoD/blockchain-explorer/actions/workflows/gitleaks.yml/badge.svg)](https://github.com/CryptoD/blockchain-explorer/actions/workflows/gitleaks.yml)
[![codecov](https://codecov.io/gh/CryptoD/blockchain-explorer/branch/main/graph/badge.svg)](https://codecov.io/gh/CryptoD/blockchain-explorer)

A comprehensive web-based application for exploring the Bitcoin blockchain with real-time access to blocks, transactions, and address information.

## Table of Contents
- [Features](#features)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Running the Application](#running-the-application)
- [API Documentation](#api-documentation)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Technology Stack](#technology-stack)
  - [Development Setup](#development-setup)
  - [Code style (Go)](#code-style-go)
  - [Code coverage](#code-coverage)
  - [Race detector (CI)](#race-detector-ci)
  - [Benchmarks](#benchmarks)
  - [E2E tests (Playwright)](#e2e-tests-playwright)
  - [Mutation testing (optional)](#mutation-testing-optional)
  - [Chaos testing (optional)](#chaos-testing-optional)
  - [Security scanning (CI)](#security-scanning-ci)
  - [Dependency updates (policy)](#dependency-updates-policy)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Features

### Search Functionality
- **Block Search**: Search by block height or block hash
- **Transaction Search**: Search by transaction hash (TXID)
- **Address Search**: Search by Bitcoin address
- **Autocomplete**: Real-time suggestions as you type

### Financial News & Contextual Data
- **Provider**: TheNewsAPI (`https://www.thenewsapi.com/`)
- **Why this provider**:
  - Clear plan-based quotas (Free: 100 requests/day; paid tiers scale to thousands/day)
  - Supports symbol/keyword filtering via a structured `search` query parameter (AND/OR/NOT, phrases, grouping)
- **Configuration**: copy `.env.example` to `.env` and set:
  - `NEWS_PROVIDER=thenewsapi`
  - `THENEWSAPI_API_TOKEN=...`
  - `THENEWSAPI_BASE_URL=https://api.thenewsapi.com`
  - Optional defaults: `THENEWSAPI_DEFAULT_SEARCH`, `THENEWSAPI_DEFAULT_LANGUAGE`, `THENEWSAPI_DEFAULT_LOCALE`, `THENEWSAPI_DEFAULT_CATEGORIES`

### Real-time Data Display
- **Latest Blocks**: View the most recent blocks with key statistics
- **Latest Transactions**: See recent transaction activity
- **Network Status**: Current network statistics and metrics

### User Interface Features
- **Responsive Design**: Works on desktop, tablet, and mobile devices
- **Internationalization**: Multi-language support
- **Dark/Light Theme**: Comfortable viewing in different environments
- **Keyboard Navigation**: Full keyboard accessibility support
- **Feedback Form**: Users can submit feedback directly from the homepage

## Architecture

The Bitcoin Explorer follows a microservices-inspired architecture with clear separation of concerns:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │────▶│   Backend API   │────▶│  Blockchain     │
│  (HTML/JS/CSS)  │     │   (Go/Gin)      │     │  Node/API       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                        │                        │
        ▼                        ▼                        ▼
┌─────────────────┐     ┌─────────────────┐
│   CDN/Static    │     │   Redis Cache   │
│   Assets        │     │   (Caching)     │
└─────────────────┘     └─────────────────┘
```

### Key Components
- **Frontend**: Single-page application with responsive design
- **Backend API**: RESTful API built with Go and Gin framework
- **Cache Layer**: Redis for performance optimization
- **Blockchain Integration**: Direct RPC connection to Bitcoin node

## Getting Started

### Prerequisites
- Go 1.22+
- Redis 6+
- Node.js 16+ (for frontend development)
- Bitcoin node with RPC access (or access to a blockchain API)

### Installation
1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd bitcoin-explorer
   ```

2. Copy environment file and configure:
   ```bash
   cp .env.example .env
   # Edit .env and set required values:
   # - POSTGRES_PASSWORD
   # - GETBLOCK_BASE_URL / GETBLOCK_ACCESS_TOKEN
   # - ADMIN_USERNAME / ADMIN_PASSWORD (especially for non-development environments)
   ```

3. Install frontend dependencies:
   ```bash
   npm install
   ```

4. Start the application with Docker Compose:
   ```bash
   docker-compose up -d
   ```

The application will start on `http://localhost:3000`, with Adminer available at `http://localhost:8080` for database administration.

### Alternative: Manual Setup

1. Install Go dependencies:
   ```bash
   go mod tidy
   go mod download
   ```

2. Configure environment variables:
   ```bash
   export APP_ENV=development                               # or staging/production
   export GETBLOCK_BASE_URL="https://your.bitcoin.node.endpoint"
   export GETBLOCK_ACCESS_TOKEN="your-api-key"
   export REDIS_URL="redis://localhost:6379"
   export ADMIN_USERNAME="admin"                            # for local dev only
   export ADMIN_PASSWORD="change_me_admin_password"         # for local dev only
   ```

3. Run the application:
   ```bash
   go run ./cmd/server
   ```

The application will start on `http://localhost:8080`

## API Documentation

For detailed API documentation, see [API_TEST_RESULTS.md](API_TEST_RESULTS.md)

### Quick API Reference

Base URL (versioned): `http://localhost:8080/api/v1`

#### Common query parameters

Many list-style endpoints accept standardized pagination and sorting parameters:

- **`page`**: 1-based page index (default: `1`, minimum: `1`).
- **`page_size`**: number of items per page (default: `20`, maximum: `100`).
- **`sort_by`** / **`sort_dir`**:
  - For advanced search: `sort_by` in `{symbol,name,type,category,market_cap,price,volume_24h,change_24h,rank,listed_since}`, `sort_dir` in `{asc,desc}`.
  - For portfolio listing: `sort_by` in `{created,updated}`, `sort_dir` in `{asc,desc}`.
  - Invalid or unsupported values fall back to the documented defaults.

All endpoints that can return large lists **cap `page_size` at 100** to protect the service and ensure predictable performance.

#### Search Endpoints
- `GET /api/v1/search?q={query}` - Search blocks, transactions, or addresses
- `GET /api/v1/search/advanced` - Advanced symbol search with filters/sorting/pagination
- `GET /api/v1/search/categories` - Retrieve available symbol categories

#### Response Format
All responses are in JSON format:

Success Response:
```json
{
  "success": true,
  "data": { ... },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

Error Response:
```json
{
bitcoin-explorer/
├── cmd/server/             # Program entry (`main`); thin wiring
├── internal/server/        # HTTP server, routes, handlers (application logic)
├── go.mod                  # Go module definition
├── go.sum                  # Go dependencies
├── bitcoin.html            # Main frontend file
├── index.html              # Alternative frontend
├── src/                    # Frontend source files
│   └── styles/             # Stylesheets
├── dist/                   # Built assets
├── docker-compose.yml      # Docker Compose configuration
├── Dockerfile              # Docker build configuration
├── package.json            # Frontend dependencies
├── API_TEST_RESULTS.md     # API testing documentation
├── README.md               # This file
└── LICENSE                 # License file
```

## Development

### Project Structure
```
bitcoin-explorer/
├── cmd/server/             # Program entry (`main`); thin wiring
├── internal/server/        # HTTP server, routes, handlers (application logic)
├── go.mod                  # Go module definition
├── go.sum                  # Go dependencies
├── bitcoin.html            # Main frontend file
├── index.html              # Alternative frontend
├── src/                    # Frontend source files
│   ├── components/         # Reusable UI components
│   ├── pages/              # Page components
│   ├── services/           # API service clients
│   └── utils/              # Utility functions
├── internal/               # Internal packages
│   ├── api/                # API handlers and routes
│   ├── blockchain/         # Blockchain integration
│   ├── cache/              # Caching layer (Redis)
│   ├── config/             # Configuration management
│   ├── blockchain/         # Blockchain integration (Bitcoin Core RPC)
│   └── utils/              # Internal utilities
├── docs/                   # Documentation files
├── scripts/                # Build and deployment scripts
└── tests/                  # Test files
```

### Technology Stack
- **Backend**: Go with Gin framework
- **Frontend**: HTML, CSS (Tailwind), JavaScript
- **Database**: PostgreSQL
- **Cache**: Redis
- **Blockchain Integration**: Bitcoin Core RPC
- **Deployment**: Docker, Kubernetes (optional)

### Development Setup

1. Install development dependencies:
   ```bash
   # Go formatting / imports (match CI versions)
   go install golang.org/x/tools/cmd/goimports@v0.30.0
   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4

   # Install frontend dependencies
   npm install
   ```

2. Run tests:
   ```bash
   go test ./...
   ```

3. Run development server with hot reload:
   ```bash
   go run ./cmd/server
   ```

### Load testing

[k6](https://k6.io/) and [Vegeta](https://github.com/tsenart/vegeta) scripts live under [`scripts/loadtest/`](scripts/loadtest/) (`k6.js`, `run-vegeta.sh`). Record baseline latency and error rates in [`scripts/loadtest/BASELINE.md`](scripts/loadtest/BASELINE.md) after runs on a consistent environment.

The server applies **gzip** (and optionally **brotli** when clients advertise `br`) to JSON/HTML/text responses by default; set `RESPONSE_COMPRESSION_ENABLED=false` if a reverse proxy already compresses. Compare encoder CPU cost with `go test ./internal/server -bench=BenchmarkCompressLargeJSON -benchmem -run=^$`.

### Code style (Go)

CI runs **`gofmt -s`**, **`goimports`**, and **[golangci-lint](https://golangci-lint.run/)** using [`.golangci.yml`](.golangci.yml) (standard preset). Formatting or lint failures fail the build.

Before committing Go changes:

```bash
gofmt -s -w .
goimports -w .
golangci-lint run ./...
go test ./...
```

Ensure `$(go env GOPATH)/bin` is on your `PATH` so `goimports` and `golangci-lint` resolve.

### Code coverage

Generate a merged profile and an HTML report (same flags as CI):

```bash
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

`coverage.out` and `coverage.html` are gitignored.

**Per-package floors (gradual ratchet):** [`scripts/coverage_thresholds.txt`](scripts/coverage_thresholds.txt) lists minimum statement coverage for selected packages (for example `internal/news` and `internal/server`). CI runs [`scripts/check_coverage.sh`](scripts/check_coverage.sh) after tests; raise a threshold when you improve coverage so it cannot regress. Long-term targets in the roadmap are on the order of **≥60%** for news and **≥40%** for the server package—current floors are set just below measured coverage until tests catch up.

On pull requests, [Codecov](https://codecov.io) can enforce a **minimum coverage on new/changed lines** (patch coverage in `codecov.yml`). To enable uploads and the README badge, add a repository secret **CODECOV_TOKEN** from your Codecov project settings and ensure the repo slug in Codecov matches `CryptoD/blockchain-explorer` (or adjust badge URLs). If the secret is absent, CI still runs tests and uploads `coverage.out` / `coverage.html` as workflow artifacts.

### Race detector (CI)

The [Race workflow](.github/workflows/race.yml) runs **`go test -race -count=1 ./...`** on every push to `main` / `master`, weekly (Mondays 06:00 UTC), and via **workflow dispatch**. Run the same command locally before merging concurrency-sensitive changes:

```bash
go test -race -count=1 ./...
```

### Benchmarks

Hot-path benchmarks live in [`internal/repos/bench_test.go`](internal/repos/bench_test.go) (Redis key builders) and [`internal/server/bench_test.go`](internal/server/bench_test.go) (cached blockchain search, large JSON payloads). Run:

```bash
go test ./internal/repos ./internal/server -bench=. -benchmem -run=^$
```

### E2E tests (Playwright)

Browser tests live in [`e2e/`](e2e/) ([`playwright.config.ts`](playwright.config.ts)). They cover **login** (development admin), **home search validation**, and **portfolios** UI. CI runs them in [`.github/workflows/e2e.yml`](.github/workflows/e2e.yml) against a real server with Redis and placeholder `GETBLOCK_*` env.

Local run (Redis required on `localhost:6379`, CSS built, `GETBLOCK_BASE_URL` and `GETBLOCK_ACCESS_TOKEN` set, server listening on `PLAYWRIGHT_BASE_URL`):

```bash
npm run build:css
go build -o explorer ./cmd/server
APP_ENV=development REDIS_HOST=localhost GETBLOCK_BASE_URL=https://example.invalid/ GETBLOCK_ACCESS_TOKEN=test HTTP_LISTEN_ADDR=127.0.0.1:8080 ./explorer &
npx playwright install chromium
PLAYWRIGHT_BASE_URL=http://127.0.0.1:8080 npm run test:e2e
```

Production Docker and CDN-oriented builds run **`npm run build`** (CSS + stamped HTML under `build/stamped/`). See [docs/CDN_STATIC_ASSETS.md](docs/CDN_STATIC_ASSETS.md) for versioned URLs, **`STATIC_ASSET_CACHE_MAX_AGE_SECONDS`**, and **`CDN_BASE_URL`**.

### Mutation testing (optional)

[Gremlins](https://github.com/go-gremlins/gremlins) runs mutation testing on small, mostly pure packages to find gaps in unit tests. The repo provides [`scripts/mutation_test.sh`](scripts/mutation_test.sh) for **`internal/apperrors`**, **`internal/correlation`**, and **`internal/apiutil`** (fuzz test files are excluded). Install the CLI pinned to **v0.5.1** (compatible with the project Go toolchain), then run from the repository root:

```bash
go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1
./scripts/mutation_test.sh
```

Interpret **LIVED** and **NOT COVERED** mutants as hints to add or tighten tests. There is no efficacy gate in CI. To run the same checks in GitHub Actions, use **Actions → Mutation → Run workflow** ([`.github/workflows/mutation.yml`](.github/workflows/mutation.yml)).

### Chaos testing (optional)

For **staging** or local resilience drills, [Toxiproxy](https://github.com/Shopify/toxiproxy) can sit in front of Redis while you add latency, timeouts, or resets via its HTTP API. The repo includes [`scripts/chaos/docker-compose.yml`](scripts/chaos/docker-compose.yml) (Redis + Toxiproxy; proxied Redis on host port **6380**, API on **8474**). Point the app at the proxy with **`REDIS_HOST`** and **`REDIS_PORT`** (defaults remain `localhost` / `6379`). Full instructions: [`docs/CHAOS_TESTING.md`](docs/CHAOS_TESTING.md).

### Dependency updates (policy)

**[GitHub Dependabot](https://docs.github.com/en/code-security/dependabot)** opens **monthly**, **grouped** pull requests for Go modules, npm, and GitHub Actions (see [`.github/dependabot.yml`](.github/dependabot.yml)). Review and merge those PRs when **CI is fully green**, including **gosec** and **Trivy** (filesystem + Docker image)—see [Dependency update policy](docs/DEPENDENCY_UPDATES.md).

### Security scanning (CI)

CI includes a dedicated **[Gitleaks](https://github.com/gitleaks/gitleaks)** workflow ([`.github/workflows/gitleaks.yml`](.github/workflows/gitleaks.yml)) on every push and pull request to `main` / `master` (full git history). It uses the repo [`.gitleaks.toml`](.gitleaks.toml) (default rules plus a small allowlist for a legacy placeholder string). Any finding fails the job.

The main **CI** workflow also runs:

| Tool | What it checks |
|------|------------------|
| **[gosec](https://github.com/securego/gosec)** | Go source for common security mistakes (`gosec ./...`). |
| **[Trivy](https://github.com/aquasecurity/trivy)** (filesystem) | Known **vulnerabilities** in dependencies (`go.sum`, npm lockfile, etc.); skips `node_modules`, build output, and `.git`. |
| **Trivy** (image) | Vulnerabilities in the **built Docker image** (`blockchain-explorer:latest`) after `docker build`. |

For **gosec** and **Trivy**, failures are reported in the **job log** (table output) and fail the workflow for **HIGH** and **CRITICAL** severities with **fixes available** (`ignore-unfixed: true` reduces noise from unfixed upstream CVEs).

Run similar checks locally:

```bash
docker run --rm -v "$PWD":/work -w /work zricethezav/gitleaks:v8.24.2 detect --source . --config .gitleaks.toml

go install github.com/securego/gosec/v2/cmd/gosec@v2.22.10
gosec -fmt text -stdout ./...

docker run --rm -v "$PWD":/work -w /work aquasec/trivy:latest fs \
  --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed \
  --skip-dirs node_modules,.git,dist,build,.tmp,vendor .

docker build -t blockchain-explorer:latest .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock aquasec/trivy:latest image \
  --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed blockchain-explorer:latest
```

### Admin credentials, environments, and rotation

- In **development** (`APP_ENV=development` or unset), the application will fall back to `admin` / `admin123` if `ADMIN_USERNAME` / `ADMIN_PASSWORD` are not provided. This is for local convenience only and must not be used in shared or production deployments.
- In any **non-development** environment (`APP_ENV` not equal to `development`), the app will **refuse to start** if `ADMIN_USERNAME` or `ADMIN_PASSWORD` are missing. Set these to strong, unique values and rotate them by updating the environment, restarting the app, and then changing the password through the UI if desired.

## Documentation

- [Bounded contexts](docs/BOUNDED_CONTEXTS.md) - domains (auth, explorer, portfolio, watchlist, news, alerts, admin), dependencies, and anti-patterns
- [Internal APIs & stability](docs/INTERNAL_APIS.md) - `internal/` package tiers (Stable / Evolving / Shell) and contracts for future extraction
- [Chaos testing (Toxiproxy)](docs/CHAOS_TESTING.md) - optional Redis flakiness drills in staging
- [Threat model (STRIDE-lite)](docs/THREAT_MODEL.md) - assets, actors, mitigations
- [Dependency update policy](docs/DEPENDENCY_UPDATES.md) - Dependabot schedule, monthly review, Trivy-aligned merges
- [SMTP TLS](docs/SMTP_TLS.md) - verified TLS in production; `SMTP_SKIP_VERIFY` only in development (with startup warning)
- [Security HTTP headers](docs/SECURITY_HEADERS.md) - CSP, HSTS, framing, env vars (`HSTS_*`)
- [CSRF and sessions](docs/CSRF_AND_SESSIONS.md) - session TTL, CSRF rotation on password change, tests
- [Rate limits and probe exemptions](docs/RATE_LIMITS.md) - global limits, `/health`/`/ready` (and `/healthz`/`/readyz` aliases), `/metrics` + `METRICS_RATE_LIMIT_PER_IP`
- [Health vs. readiness](docs/HEALTH_AND_READINESS.md) - liveness (`/health`) vs readiness (`/ready`), Kubernetes probes
- [SLOs and error budget](docs/SLO_AND_ERROR_BUDGET.md) - search latency targets (e.g. 99% &lt; 500 ms), Prometheus examples, budget policy
- [Outbound circuit breakers](docs/CIRCUIT_BREAKERS.md) - per-host gobreaker on RPC/pricing/news HTTP, half-open recovery, env tuning
- [Retry budgets](docs/RETRY_BUDGET.md) - Resty retry cap and optional per-inbound outbound attempt budget
- [Idempotency keys (exports)](docs/IDEMPOTENCY_KEYS.md) - `Idempotency-Key` header, JSON replay and streaming de-dupe
- [Input size limits](docs/INPUT_LIMITS.md) - max body, JSON depth, CSV export row caps
- [SQL (N/A) and Redis key safety](docs/SQL_AND_REDIS_SAFETY.md) - no SQL in app; Redis keys, SCAN patterns, future SQL/Lua guidance
- [Redis backup and restore](docs/REDIS_BACKUP_AND_RESTORE.md) - RDB/AOF, managed vs self-hosted, restore outline, data lost on total loss
- [Disaster recovery drill (quarterly)](docs/DISASTER_RECOVERY_DRILL.md) - simulated Redis wipe, restore from backup, validation checklist, record template
- [API Test Results](API_TEST_RESULTS.md) - API testing and validation results
- [Security Policy](SECURITY.md) - how to report vulnerabilities and disclosure guidelines
- [External penetration testing](docs/PENETRATION_TESTING.md) - cadence (annual / pre-launch), tracking findings in issues

## Contributing

We welcome contributions to improve the Bitcoin Explorer. Please follow standard Go and web development practices.

Pull requests are checked in GitHub Actions: **Go** code must satisfy **gofmt** (simplified), **goimports**, **golangci-lint**, and tests; **Gitleaks**, **gosec**, and **Trivy** (filesystem + Docker image) must pass; **frontend** CSS build must succeed; **Docker** image must build and scan clean. Pushes to the default branch also run the [race detector workflow](#race-detector-ci). The [E2E workflow](#e2e-tests-playwright) runs Playwright against the built server. **Dependabot** dependency PRs follow [Dependency updates (policy)](#dependency-updates-policy). Run the commands in [Code style (Go)](#code-style-go), [Security scanning (CI)](#security-scanning-ci), and `go test ./...` locally before opening a PR.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.