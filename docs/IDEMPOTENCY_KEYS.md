# Idempotency keys (exports)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **57**. Clients may send an **`Idempotency-Key`** header on **export** endpoints so **safe replays** (retries, double-clicks) do not multiply work or return divergent bodies for the same logical export.

**Implementation:** [`internal/idempotency`](../internal/idempotency/) (Redis-backed), wired in [`Run`](../internal/server/run.go) as [`idempotency.NewStore`](../internal/idempotency/store.go), invoked from [`beginExportIdempotency`](../internal/server/export_idempotency.go) on export handlers in [`searchblockchain.go`](../internal/server/searchblockchain.go) and [`updatewatchlistentryhandler.go`](../internal/server/updatewatchlistentryhandler.go).

---

## Behavior

| Export type | `Idempotency-Key` present | Result |
|-------------|---------------------------|--------|
| **JSON** (`/search/export`, `/search/advanced/export`, `/portfolios/export`) | First success | Response bytes stored in Redis (size-capped). |
| **JSON** | Later replay (same key + same request fingerprint) | **200** with identical body; header **`Idempotent-Replayed: true`**. |
| **Streaming** (CSV/PDF block/tx/portfolio file) | First success | Redis records “stream completed” for this key + fingerprint. |
| **Streaming** | Later replay | **409** with `code: idempotency_replay` (no second full generation). |

**Fingerprint** = HTTP method + path + **sorted** query parameters so the same export intent maps to one key. **Scope** = `user:<username>` when authenticated, else `anon:<client IP>`.

**Payments:** There are no payment endpoints in this codebase today; the same store and header can be reused for future **POST** charge APIs by calling the idempotency package from those handlers.

---

## Configuration

| Env | Default | Meaning |
|-----|---------|---------|
| `IDEMPOTENCY_ENABLED` | `true` | Set `false` / `0` to disable (no Redis writes). |
| `IDEMPOTENCY_TTL_SECONDS` | `86400` | How long replay metadata is kept (60s–7d). |
| `IDEMPOTENCY_MAX_RESPONSE_BYTES` | `262144` | Max stored JSON body for JSON exports (1 KiB–10 MiB). |
| `IDEMPOTENCY_KEY_MAX_RUNES` | `128` | Max length of `Idempotency-Key` (8–256 runes). |

Validated in [`internal/config/config.go`](../internal/config/config.go).

---

## Client guidance

1. Generate a **unique key per logical export** (e.g. UUID) when starting a download; **reuse** that key only when **retrying the same** export.
2. Expect **409** on streaming replay; fetch the first successful response or use a **new** key for a new export.
3. JSON replays return the **cached** payload (timestamps inside `export_meta` reflect the **original** export time).

---

## Related

- [RETRY_BUDGET.md](RETRY_BUDGET.md) — outbound retry limits  
- [RATE_LIMITS.md](RATE_LIMITS.md) — export rate limits still apply  
