# REST write semantics — PUT/PATCH idempotency and concurrency

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **69**. It describes how **state-changing** REST endpoints behave today — especially **`PUT`** on **portfolios** and **watchlists** — and how that relates to **idempotency**, the JSON **`updated`** timestamps, **ETags**, and optional **future** optimistic locking.

**Implementation references:** portfolio [`updatePortfolioHandler`](../internal/server/feedbackhandler.go), watchlist [`updateWatchlistHandler`](../internal/server/feedbackhandler.go), profile [`updateProfileHandler`](../internal/server/updateprofilehandler.go); conditional caching for reads in [`writeJSONConditional`](../internal/server/conditional_get.go).

---

## Portfolio `PUT /api/v1/user/portfolios/{id}`

### Semantics

- The handler loads the portfolio for the authenticated user and path **`id`**, then replaces **`name`**, **`description`**, and **`items`** from the JSON body. **`id`**, **`username`**, and **`created`** stay as stored; **`updated`** is set to the server clock on every successful save.
- The **`id` (and `username`) fields in the request body are not used** to select the resource — the path parameter **`{id}`** is authoritative. Clients should send a body consistent with that id or omit those fields; the server overwrites from storage where needed.

### Idempotency (retries and duplicate requests)

- **Logical idempotency:** Re-sending the **same** intended `name` / `description` / `items` after a success applies the same business state again. Relevant for **network retries**: a duplicate `PUT` does not create a second resource (the id is fixed).
- **Response bodies are not byte-stable across retries:** Each successful write sets a **new** **`updated`** timestamp, so two identical requests produce two **200** responses with different **`updated`** values.
- There is **no** `Idempotency-Key` processing on this route (that header is for [export endpoints](IDEMPOTENCY_KEYS.md)).

### Concurrency and version fields

- **Last-write-wins:** There is **no** `If-Match` / **`If-Unmodified-Since`** check and **no** ETag on portfolio **GET**/**list**/**PUT** responses in the current implementation. Concurrent `PUT` requests from two tabs or clients are not merged; **whichever completes last** determines the stored document.
- **`updated` (JSON):** ISO-8601 / RFC3339 timestamp of the last successful save. Clients **may** use it for **UI merge hints** (e.g. “portfolio changed elsewhere — reload?”) by comparing to the value last seen, but the server **does not** treat it as a revision token and **does not** reject stale writes.

### ETags

- Many **read-only GET** JSON endpoints use **`ETag`** + **`If-None-Match`** for **conditional GET** and caching (see [`writeJSONConditional`](../internal/server/conditional_get.go)). **Portfolios are not wired through that helper** for list/detail/update responses today, so clients should not rely on `ETag` for portfolio documents.

---

## Watchlist `PUT /api/v1/user/watchlists/{id}`

- **Full replacement** of **`name`** and **`entries`** after loading the existing watchlist (identity and timestamps preserved as implemented). Same **last-write-wins** and **no** conditional headers as portfolios. **`updated`** advances on every successful save.

---

## Profile `PATCH /api/v1/user/profile`

- **Partial update:** Only fields present in the JSON body are applied. This is **not** the same as portfolio `PUT` full-replace semantics.

---

## Future: optimistic locking (not implemented)

If the API later adds **conflict-safe** updates for portfolios (or watchlists), a likely shape would be:

1. **GET** responses include a **strong `ETag`** (or a dedicated revision field) representing the current document.
2. Clients send **`If-Match: "<etag>"`** (or a body field such as `expected_updated`) on **`PUT`**.

Until that exists, integrations must assume **last-write-wins** or implement their own coordination (e.g. application-level locks or user workflow).

---

## Summary

| Resource | Method | Replace vs patch | Server revision token | Concurrent writes |
|----------|--------|------------------|------------------------|---------------------|
| Portfolio | `PUT` | Full replace of `name`, `description`, `items` | **`updated`** (informational only) | Last-write-wins |
| Watchlist | `PUT` | Full replace of `name`, `entries` | **`updated`** (informational only) | Last-write-wins |
| User profile | `PATCH` | Partial | N/A in current error responses | Last-write-wins |

For export idempotency (`Idempotency-Key`), see [IDEMPOTENCY_KEYS.md](IDEMPOTENCY_KEYS.md).
