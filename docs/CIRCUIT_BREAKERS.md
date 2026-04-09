# Outbound HTTP circuit breakers

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **55**. The shared outbound `http.Transport` used by **GetBlock JSON-RPC**, **CoinGecko** (pricing), and **news** HTTP clients is wrapped with **per-host** circuit breakers ([`github.com/sony/gobreaker`](https://github.com/sony/gobreaker)): **closed → open → half-open → closed** recovery without a single global breaker for all upstreams.

**Wiring:** [`internal/server/outbound_wire.go`](../internal/server/outbound_wire.go) → [`outboundbreaker.WrapRoundTripper`](../internal/outboundbreaker/transport.go).

---

## Behavior

| Signal | Treated as |
|--------|------------|
| **Network / TLS errors**, timeouts | Failure |
| **HTTP 5xx** | Failure (response body discarded) |
| **HTTP 4xx** (including 429) | Success for breaker purposes (upstream is “up”; rate limits are not misinterpreted as a dead host) |

When failures exceed the trip threshold, the breaker for that **host** (from `request.URL.Host`) moves to **open**: further requests fail fast with `circuit breaker is open` until the **open timeout** elapses, then **half-open** allows a limited number of probe requests. Success closes the breaker; failure reopens it.

**Default trip rule** (when `OUTBOUND_CIRCUIT_BREAKER_TRIP_AFTER_CONSECUTIVE_FAILURES=0`): gobreaker trips when **consecutive failures &gt; 5** (i.e. the **6th** consecutive failure transitions to open). Override with a positive integer to use **≥ N** consecutive failures instead.

---

## Configuration (environment)

| Env | Default | Meaning |
|-----|---------|---------|
| `OUTBOUND_CIRCUIT_BREAKER_ENABLED` | `true` (unset or empty = on) | Set `false` or `0` to disable wrapping. |
| `OUTBOUND_CIRCUIT_BREAKER_OPEN_TIMEOUT_SECONDS` | `60` | How long the breaker stays **open** before trying **half-open**. |
| `OUTBOUND_CIRCUIT_BREAKER_INTERVAL_SECONDS` | `0` | In the **closed** state, reset internal `Counts` every N seconds; `0` = never (gobreaker default). |
| `OUTBOUND_CIRCUIT_BREAKER_HALF_OPEN_MAX_REQUESTS` | `1` | Max concurrent requests allowed in **half-open**. |
| `OUTBOUND_CIRCUIT_BREAKER_TRIP_AFTER_CONSECUTIVE_FAILURES` | `0` | Trip when consecutive failures ≥ N; `0` = library default (&gt; 5). Valid range 0–100. |

Validated in [`internal/config/config.go`](../internal/config/config.go) (`validateOutboundCircuitBreaker`).

---

## Metrics

When `METRICS_ENABLED` is on, Prometheus exposes:

- `explorer_outbound_circuit_breaker_transitions_total{host,from_state,to_state}`
- `explorer_outbound_circuit_breaker_rejections_total{host,reason}` — `reason` is `open` or `half_open`

`host` is the HTTP `Host` header value (truncated for the metric label). Cardinality is usually **one breaker per external API** (GetBlock, CoinGecko, news).

---

## Out of scope

- **Readiness** checks that use a **separate** short-lived Resty client ([`readinessHandler`](../internal/server/getusdperfiat.go) when `READY_CHECK_EXTERNAL=true`) do **not** use this wrapper.
- **SMTP** uses the standard library, not the shared outbound transport.

---

## Related

- Retry behavior is configured via **`OUTBOUND_HTTP_RETRY_COUNT`** (Resty `SetRetryCount` in [`newRestyClientForConfig`](../internal/server/outbound_wire.go)); optional per-inbound caps in [RETRY_BUDGET.md](RETRY_BUDGET.md). Circuit **open** fails before the network on that host, so retries do not amplify load against a tripped upstream.
