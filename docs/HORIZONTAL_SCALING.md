# Horizontal scaling checklist (N replicas)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **51**. It describes what **already works** behind a load balancer with **shared Redis**, what **does not** today, and which **environment variables** must be consistent across pods.

## TL;DR

| Concern | Shared Redis + N replicas | Notes |
|--------|---------------------------|--------|
| **Session + CSRF** | **OK without sticky sessions** | Session id and CSRF live in Redis ([`SessionRepo`](../internal/repos/session.go)); any replica can validate the cookie. |
| **Portfolios, watchlists, alerts, notifications, feedback** | **OK** | Data keys are in Redis ([`internal/repos/keys.go`](../internal/repos/keys.go)). |
| **User accounts (register / login / profile)** | **Not safe as implemented** | The process keeps a **local `users` map** loaded at startup ([`loadUsersFromRedis`](../internal/server/init.go)); new users and password updates are written to Redis **and** memory on the handling pod only. Other pods do not reload—**login on another replica can fail** until code reads through Redis or invalidates cache. |
| **Rate limiting** | **Mostly OK** with Redis | Per-IP / per-user limits use Redis when available. **In-memory fallback** ([`rateLimitMiddleware`](../internal/server/updateprofilehandler.go)) is **per pod**—effective limits scale ~linearly with replica count (weaker per-IP cap) or behave oddly if Redis flaps. |
| **Background jobs** | **Duplicated per replica** | Prefetch, chart metrics collection, and **price-alert evaluation** run inside each process ([`run.go`](../internal/server/run.go)). N replicas ⇒ N× Redis scans / upstream work unless you add **leader election** or run jobs as a **separate singleton** deployment. |

## Session stickiness vs shared Redis

- **Preferred:** **No stickiness** when **Redis is up** and all app instances use the **same** `REDIS_HOST` / `REDIS_PORT`. Session and CSRF validation hit Redis on every request path that needs them.
- **Sticky sessions** do **not** replace Redis: they only help if you deliberately run **without** shared Redis (not recommended) or during a misconfiguration where only in-memory session fallback is active—in that case traffic must stay on the pod that issued the cookie.
- **If Redis is unavailable**, the app falls back to **in-process** session and CSRF stores ([`validateSession`](../internal/server/init.go)). Those are **not shared**; you would need **strict stickiness** or **a single replica** until Redis recovers.

## Required environment (identical or equivalent across replicas)

Set the same values on **every** app pod (Kubernetes Deployment, ECS tasks, etc.):

| Variable | Why |
|----------|-----|
| **`REDIS_HOST`**, **`REDIS_PORT`** | Single logical Redis (or Sentinel/Cluster address your client supports). All session, user, portfolio, rate-limit, and cache data must converge here. |
| **`GETBLOCK_BASE_URL`**, **`GETBLOCK_ACCESS_TOKEN`** | Shared upstream; avoids per-pod drift. |
| **`APP_ENV`** | Consistent behavior (cookies, validation, Sentry sampling). |
| **`ADMIN_USERNAME`**, **`ADMIN_PASSWORD`** (non-dev) | Same admin identity across pods (stored in Redis after first init). |
| **Auth / integration secrets** | `SENTRY_DSN`, SMTP, news API, email provider keys—must match if you want uniform behavior. |
| **`SECURE_COOKIES`**, **`HSTS_*`** | Align with how TLS terminates (load balancer vs ingress vs app). |
| **`CDN_BASE_URL`** (if used) | Same as build-time stamp + CSP ([`CDN_STATIC_ASSETS.md`](CDN_STATIC_ASSETS.md)). |

**Optional but common behind a proxy:**

- **`HTTP_LISTEN_ADDR`** / **`APP_PORT`** — per-pod bind address; often `:8080` inside the container while the Service maps 80→8080.
- Trust **`X-Forwarded-Proto`** for HSTS and secure cookies—ensure your ingress sets it when TLS terminates at the edge ([`security_headers.go`](../internal/server/security_headers.go)).

## Redis sizing and availability

- Use one **highly available** Redis (managed HA, Sentinel, or Cluster per your ops standard). **Split-brain** or wrong endpoint per region breaks sessions instantly.
- Connection pool env vars apply **per pod** ([`internal/config/config.go`](../internal/config/config.go)): `REDIS_POOL_SIZE`, `REDIS_MAX_ACTIVE_CONNS`, etc. Total connections ≈ **pool × replicas**—size Redis `maxclients` accordingly.

## Before running N replicas in production

1. **Confirm Redis is mandatory** for sessions in prod (no reliance on in-memory session fallback).
2. **Acknowledge user-map limitation** or **fix auth** to load/refresh users from Redis on cache miss (future code change—not done in task 51).
3. **Decide on background work**: accept duplicate jobs, scale replicas to 1 for the worker, or introduce a **cron / separate Deployment** with `replicas: 1` for evaluators.
4. **Load balancer health checks**: `GET /health` / `GET /ready` (or `GET /healthz` / `GET /readyz` aliases; see [`HEALTH_AND_READINESS.md`](HEALTH_AND_READINESS.md), [`RATE_LIMITS.md`](RATE_LIMITS.md))—ensure probes do not share the same rate-limit bucket as abusive clients if you tune limits down.
5. **Metrics**: each pod exposes `/metrics` if enabled—scrape all targets or use a DaemonSet/sidecar pattern your observability stack expects.

## Related documents

- [CSRF_AND_SESSIONS.md](CSRF_AND_SESSIONS.md) — session and CSRF lifecycle.
- [SQL_AND_REDIS_SAFETY.md](SQL_AND_REDIS_SAFETY.md) — Redis key layout.
- [REDIS_BACKUP_AND_RESTORE.md](REDIS_BACKUP_AND_RESTORE.md) — RDB/AOF, backups, what is lost on total Redis loss.
- [DISASTER_RECOVERY_DRILL.md](DISASTER_RECOVERY_DRILL.md) — quarterly simulated wipe + restore drill.
- [FEATURE_FLAGS.md](FEATURE_FLAGS.md) — toggle news and price alerts via env or Redis without redeploy.
- [THREAT_MODEL.md](THREAT_MODEL.md) — trust boundaries.
- [POSTGRES_MIGRATION_SKETCH.md](POSTGRES_MIGRATION_SKETCH.md) — if you later move durable user state off Redis.
