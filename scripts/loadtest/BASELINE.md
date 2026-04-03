# Load test baselines

Record numbers here after running [`k6.js`](k6.js) or [`run-vegeta.sh`](run-vegeta.sh) against a **known environment** (same machine class, Redis, RPC, and build). Update when infrastructure or code materially changes.

## How to run

**k6** (search + login + advanced search API):

```bash
cd /path/to/blockchain-explorer
export BASE_URL=http://127.0.0.1:8080
export LOADTEST_USERNAME=admin
export LOADTEST_PASSWORD=admin123
k6 run scripts/loadtest/k6.js
```

Omit `LOADTEST_USERNAME` / `LOADTEST_PASSWORD` to skip the login scenario (still runs search + advanced).

**Vegeta** (GET-only: `/api/search`, advanced search, dashboard HTML):

```bash
BASE_URL=http://127.0.0.1:8080 ./scripts/loadtest/run-vegeta.sh
```

## Baseline table (fill in)

| Run date   | Git revision | Env (CPU/RAM) | Scenario              | Target RPS | p95 latency | http error % | Notes |
|------------|--------------|---------------|------------------------|------------|-------------|--------------|-------|
| _TBD_      | _TBD_        | _TBD_         | `api_search` (k6)     | 10/s       |             |              |       |
| _TBD_      | _TBD_        | _TBD_         | `auth_login` (k6)     | 3/s        |             |              |       |
| _TBD_      | _TBD_        | _TBD_         | `heavy_advanced_search` | 5/s      |             |              |       |
| _TBD_      | _TBD_        | _TBD_         | vegeta (combined GET) | 20/s       |             |              |       |

**Heavy path:** `GET /api/v1/search/advanced` with pagination (symbol DB + filters) — heavier than a single `/api/search` lookup.

**Dashboard path (vegeta):** `GET /dashboard` serves the SPA shell (static HTML).
