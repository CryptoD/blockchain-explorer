# Rate limiting and probe exemptions

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **32**. It describes how the global HTTP rate limiter interacts with **metrics**, **health probes**, and the rest of the API.

**Implementation:** [`rateLimitMiddleware`](../internal/server/updateprofilehandler.go), configuration in [`internal/config/config.go`](../internal/config/config.go).

---

## Global limit (default)

| Setting | Env | Default |
|---------|-----|---------|
| Window | `RATE_LIMIT_WINDOW_SECONDS` | `60` |
| Per IP | `RATE_LIMIT_PER_IP` | `10` |
| Per user (authenticated) | `RATE_LIMIT_PER_USER` | `10` |

The limiter uses **Redis** when available (`rate:ip:<clientIP>` and `rate:user:<username>`), with an **in-memory fallback** per IP if Redis is down.

---

## Paths exempt from the global limit

These routes are **not** counted against `RATE_LIMIT_PER_IP` / `RATE_LIMIT_PER_USER`:

| Path | Reason |
|------|--------|
| **`GET /health`**, **`GET /healthz`** | **Liveness** (aliases): small JSON only—**no** Redis or RPC ([`livenessHandler`](../internal/server/getusdperfiat.go)). Orchestrators often probe **more than** 10×/min per IP; exempting avoids **429** on healthy pods. |
| **`GET /ready`**, **`GET /readyz`** | **Readiness** (aliases): Redis `PING` and optionally a lightweight external RPC when `READY_CHECK_EXTERNAL=true` ([`readinessHandler`](../internal/server/getusdperfiat.go)). Same probe-frequency rationale. **Note:** readiness can do more work per request when external checks are enabled; keep intervals conservative. |

All other routes are subject to the global limit, including **`GET /api/v1/metrics`** (explorer JSON metrics) and **API search** routes.

---

## Prometheus `GET /metrics`

Prometheus scrapes are **exempt from the global** limiter so scrape intervals do not consume the same budget as the API.

**DoS mitigation:**

1. **`METRICS_TOKEN`** — If set, [`TokenAuthMiddleware`](../internal/metrics/metrics.go) requires `Authorization: Bearer <token>` or `X-Metrics-Token`. Unauthenticated scrapes are rejected with **401**; the global limit does not apply to those failed requests. **Recommended in production** so only your monitoring stack can scrape.
2. **Unauthenticated metrics** (`METRICS_ENABLED` true and **`METRICS_TOKEN` empty**) — A **separate** per-IP limit applies: **`METRICS_RATE_LIMIT_PER_IP`** (default **120** per window, same window as `RATE_LIMIT_WINDOW_SECONDS`). Keys: `rate:metrics:ip:<ip>` in Redis. Set to **`0`** to disable this sub-limit (development only; not recommended on the public internet).

---

## Operational guidance

- **Public deployments:** Set **`METRICS_TOKEN`**, restrict `/metrics` at the **load balancer** or network policy (Prometheus only), and keep **`METRICS_RATE_LIMIT_PER_IP`** at a sensible default.
- **High-frequency probes:** `/health`, `/healthz`, `/ready`, and `/readyz` are exempt from the global limit; tune **`READY_CHECK_EXTERNAL`** (and external RPC cost) if you rely on deep readiness checks.
- **Per-route limits:** Stricter limits apply to **export** endpoints (`EXPORT_RATE_LIMIT_*` in config); see [`checkExportRateLimit`](../internal/server/init.go).

---

## Related tests

[`internal/server/rate_limit_test.go`](../internal/server/rate_limit_test.go) covers health exemption, general API enforcement, and unauthenticated metrics scrape limits.
