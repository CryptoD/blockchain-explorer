# SLO definitions and error budget

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **53**. It defines **service level objectives (SLOs)** for this explorer, the **service level indicators (SLIs)** used to measure them, and how **error budgets** translate objectives into operational decisions.

These targets are **aspirational defaults** for a production-like deployment (sized Redis, stable RPC provider, TLS termination if applicable). Tune them per environment; record evidence in [`scripts/loadtest/BASELINE.md`](../scripts/loadtest/BASELINE.md) after load tests.

---

## Terms

| Term | Meaning |
|------|---------|
| **SLI** | A measurable property of service behavior (e.g. proportion of search requests completing within 500 ms). |
| **SLO** | A target for an SLI over a time window (e.g. ≥ **99%** of search requests &lt; **500 ms** over **30 days**). |
| **Error budget** | The allowable amount of “bad” events before the SLO would be violated: `(1 − SLO_target) × valid_events` in the window. Spending the budget means latency misses, errors, or downtime are consuming that allowance. |

**Note:** “**99% of requests under 500 ms**” is a **proportion** SLO (good requests ÷ total). It is closely related to the **99th percentile** (p99) when the distribution is well behaved, but they are not identical: a spike of very slow requests can burn budget without changing p99 much if volume is low. Prefer measuring **proportion under threshold** for this document’s search latency SLO.

---

## Search latency SLO (primary)

**Scope — “search” HTTP handlers** (server-side time only; `handler` label is Gin `FullPath()`):

- `GET /api/search`
- `GET /api/search/export`
- `GET /api/search/advanced`
- `GET /api/search/advanced/export`
- `GET /api/search/categories`
- `GET /api/v1/search`
- `GET /api/v1/search/export`
- `GET /api/v1/search/advanced`
- `GET /api/v1/search/advanced/export`
- `GET /api/v1/search/categories`

| SLI | SLO (default) | Window |
|-----|----------------|--------|
| Share of search requests with **end-to-end handler duration ≤ 500 ms** | **≥ 99%** | **30 rolling days** |

**Measurement:** Prometheus histogram [`http_request_duration_seconds`](../internal/metrics/metrics.go) with labels `method`, `handler`. Middleware observes wall time for the full Gin handler chain (see [`Middleware`](../internal/metrics/metrics.go)).

**Example — proportion of search traffic under 500 ms** (adjust `handler` regex to match your route list):

```promql
sum by () (
  rate(http_request_duration_seconds_bucket{
    method="GET",
    handler=~"/api/search.*|/api/v1/search.*",
    le="0.5"
  }[1h])
)
/
sum by () (
  rate(http_request_duration_seconds_count{
    method="GET",
    handler=~"/api/search.*|/api/v1/search.*"
  }[1h])
)
```

Use a **longer range** (e.g. `[30d]` on a recording rule or Grafana query) for monthly review; short windows are useful for **burn-rate** alerts (fast budget consumption).

---

## Error budget (search latency)

Let:

- **SLO target** \(T = 0.99\) (99% of requests must be “good”: duration ≤ 500 ms).
- **Bad budget fraction** = \(1 − T = 0.01\) (1% of requests may exceed 500 ms while still meeting the SLO).

Over a calendar month, if search endpoints serve **\(N\)** successful requests (200/304/3xx as counted in your numerator/denominator policy), the **error budget** for “slow” requests is about **\(0.01 × N\)** request-events. Example: **10 million** search requests ⇒ **100 000** may exceed **500 ms** without breaking the 99% SLO, if there are no other deductions.

**Operational policy (recommended):**

1. **Above 50% budget remaining** — normal feature work.
2. **25–50% remaining** — prioritize latency regressions, cache/RPC tuning, and load test baselines before large features.
3. **Below 25%** — freeze non-essential releases, focus on search path profiling ([`PPROF_ENABLED`](../internal/config/config.go)), Redis/RPC health, and capacity ([`docs/HORIZONTAL_SCALING.md`](HORIZONTAL_SCALING.md)).
4. **Budget exhausted or SLO breached** — incident-style review; consider stricter caching, rate limits, or dependency circuit-breaking (roadmap tasks **55–56**).

Document **denominator rules** in your monitoring (e.g. include only `GET`, exclude `499` client aborts if desired) so on-call and SRE agree on the numbers.

---

## Secondary SLOs (optional defaults)

These are useful companions; they are **not** a substitute for the search latency SLO above.

| Area | SLI | Suggested target | Notes |
|------|-----|------------------|--------|
| **Search availability** | Ratio of `5xx` to all responses on search routes | **&lt; 0.1%** over 30d | Use `http_requests_total` with `status=~"5.."` vs total for the same `handler` set. |
| **Readiness** | `GET /ready` or `GET /readyz` succeeds (200) when Redis and optional external checks pass | **99.9%** probe success over 30d | Orchestrator probes; see [HEALTH_AND_READINESS.md](HEALTH_AND_READINESS.md) (roadmap task **54**). |
| **Blockchain RPC** | JSON-RPC success rate | **≥ 99.5%** over 7d | [`explorer_blockchain_rpc_calls_total`](../internal/metrics/metrics.go) (`status` label). |

---

## Related artifacts

- Load test scripts and baseline table: [`scripts/loadtest/`](../scripts/loadtest/), [`BASELINE.md`](../scripts/loadtest/BASELINE.md).
- Global HTTP metrics: [`internal/metrics/metrics.go`](../internal/metrics/metrics.go).
- Multi-instance behavior: [`docs/HORIZONTAL_SCALING.md`](HORIZONTAL_SCALING.md).

---

## Review cadence

Revisit SLO targets **quarterly** or after major infra changes (new region, new RPC provider, Redis migration). If measured p95/p99 are consistently **better** than 500 ms, consider **tightening** the threshold or raising the percentage target rather than letting the bar drift upward silently.
