# Feature flags (news and price alerts)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **61**. It describes how to **disable or re-enable** the **news** and **price alert** features **without redeploying** by combining **environment defaults** with optional **Redis** overrides.

## Environment (startup)

| Variable | Default | Meaning |
|----------|---------|--------|
| `FEATURE_NEWS_ENABLED` | `true` | When `false`/`0`, symbol and portfolio news APIs return **503** with `code: feature_disabled`. |
| `FEATURE_PRICE_ALERTS_ENABLED` | `true` | When `false`/`0`, price-alert **CRUD** and background **evaluation** are skipped; APIs return **503** with `code: feature_disabled`. |

Parsing matches other booleans in the app: empty → default; `true`/`1` → on; `false`/`0` → off.

Changing env requires a **process restart** (new config load).

## Redis (runtime, no redeploy)

The server reads these **string** keys on a **shared Redis** (`REDIS_HOST` / `REDIS_PORT`). Values are cached for a few seconds per process to limit load.

| Key | Effect |
|-----|--------|
| `feature:news` | `1` / `true` / `yes` / `on` / `enabled` → news **on**; `0` / `false` / `no` / `off` / `disabled` → news **off**. |
| `feature:price_alerts` | Same semantics for price alerts. |

- If the key is **missing** (`redis.Nil`), the **env default** applies.
- If Redis is **unreachable** for a read, the resolver falls back to the **env default** for that check.
- Invalid strings fall back to the **env default**.

### Examples

Disable news immediately (CLI):

```bash
redis-cli SET feature:news 0
```

Re-enable price alerts:

```bash
redis-cli SET feature:price_alerts 1
```

Remove override (use env default again):

```bash
redis-cli DEL feature:news
```

## Admin visibility

`GET /api/v1/admin/status` (and legacy `/api/admin/status`) includes **`feature_flags`**: a small map with effective **`news`** and **`price_alerts`** booleans after Redis resolution (or env-only values if the resolver is not wired).

## Implementation

- Resolver: [`internal/featureflags`](../internal/featureflags/featureflags.go)
- HTTP gates: [`internal/server/feature_flags.go`](../internal/server/feature_flags.go), news handlers in [`updateprofilehandler.go`](../internal/server/updateprofilehandler.go), alert handlers in [`updateprofilehandler.go`](../internal/server/updateprofilehandler.go) / [`updatepricealerthandler.go`](../internal/server/updatepricealerthandler.go), background eval in [`evaluatePriceAlerts`](../internal/server/updatepricealerthandler.go)
