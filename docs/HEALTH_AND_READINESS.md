# Health vs. readiness

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **54**. It describes **liveness** and **readiness** HTTP probes, how they differ, and which paths to use in Kubernetes or other orchestrators.

**Implementation:** [`livenessHandler`](../internal/server/getusdperfiat.go), [`readinessHandler`](../internal/server/getusdperfiat.go), registration in [`registerHealthAndMetricsRoutes`](../internal/server/routes.go). Probes are exempt from the global rate limiter ([`rateLimitMiddleware`](../internal/server/updateprofilehandler.go)); see [RATE_LIMITS.md](RATE_LIMITS.md).

---

## Liveness (process is running)

**Purpose:** Answer “should this container be **restarted**?” A failing liveness probe should mean the process is stuck or deadlocked, **not** that Redis or an upstream API is unavailable.

| Path | Handler |
|------|---------|
| **`GET /health`** | [`livenessHandler`](../internal/server/getusdperfiat.go) |
| **`GET /healthz`** | Same handler (**alias** for older configs and docs) |

**Behavior:** Returns **200** with JSON `status: ok`, `timestamp`, `app_env`. **No** Redis ping, **no** GetBlock RPC, **no** dependency checks.

---

## Readiness (instance can accept traffic)

**Purpose:** Answer “should the load balancer send **traffic** to this instance?” If Redis is down or (when enabled) the blockchain RPC check fails, the instance should stop receiving traffic without being restarted.

**UI:** HTML pages poll **`GET /ready`** from [`static/js/degraded-mode.js`](../static/js/degraded-mode.js) to show a **degraded** banner when not ready ([`docs/DEGRADED_MODE_UX.md`](DEGRADED_MODE_UX.md)).

| Path | Handler |
|------|---------|
| **`GET /ready`** | [`readinessHandler`](../internal/server/getusdperfiat.go) |
| **`GET /readyz`** | Same handler (**alias**) |

**Behavior:**

1. **Redis** — `PING` must succeed; otherwise **503** with `status: not_ready`.
2. **Optional external check** — If `READY_CHECK_EXTERNAL=true` in config, a lightweight `getblockcount` JSON-RPC POST to GetBlock must succeed (short timeout, no retries). If `GETBLOCK_*` is missing when this flag is on, **503**.

Configure the external check only when you accept the extra latency and RPC load per probe; see [HORIZONTAL_SCALING.md](HORIZONTAL_SCALING.md).

---

## Kubernetes example

Use **`/health`** for `livenessProbe` and **`/ready`** for `readinessProbe` (or the `z` suffix aliases interchangeably):

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
```

Set **`terminationGracePeriodSeconds`** high enough for graceful HTTP drain; see graceful shutdown env vars (`SHUTDOWN_GRACE_SECONDS`, etc.) in the server [`Run`](../internal/server/run.go) path.

---

## Related documentation

- [RATE_LIMITS.md](RATE_LIMITS.md) — probe exemption from global limits  
- [SLO_AND_ERROR_BUDGET.md](SLO_AND_ERROR_BUDGET.md) — optional readiness SLO ideas  
- [HORIZONTAL_SCALING.md](HORIZONTAL_SCALING.md) — multi-replica probes  
