# Input size limits

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **33**. It summarizes limits on **HTTP bodies**, **JSON nesting**, and **CSV export** row caps.

| Concern | Mechanism | Configuration |
|--------|-----------|----------------|
| **Request body size** | [`requestBodyLimitsMiddleware`](../internal/server/request_limits.go) on **POST**, **PUT**, **PATCH** | `MAX_REQUEST_BODY_BYTES` (default **1048576** = 1 MiB). **0** = unlimited (not recommended on the public internet). Maximum validated: **104857600** (100 MiB). |
| **JSON nesting depth** | Same middleware for `Content-Type: application/json`; streaming parse via [`apiutil.ValidateJSONDepth`](../internal/apiutil/json_depth.go) | `MAX_JSON_DEPTH` (default **64**). **0** = skip nesting check (body size limit still applies). |
| **CSV export rows** | Block and transaction CSV handlers cap the `limit` query parameter | `EXPORT_MAX_BLOCK_CSV_ROWS` and `EXPORT_MAX_TRANSACTION_CSV_ROWS` (default **0** = use package caps only). Non-zero values **lower** the effective cap but cannot exceed [`export.MaxBlockRows`](../internal/export/meta.go) / [`export.MaxTxRows`](../internal/export/meta.go). |
| **List `page` / `page_size` (GET)** | [`apiutil.ParsePagination`](../internal/apiutil/pagination.go) on paginated list endpoints | Default **20** (`DefaultPageSize`), max **100** (`MaxPageSize`). Authenticated news portfolio/symbol lists use max **50** (`MaxPageSizeNews`). |
| **Conditional GET (ETag)** | [`writeJSONConditional`](../internal/server/conditional_get.go) on cache-friendly JSON GET handlers | Strong ETag = quoted SHA-256 hex of the exact JSON body. Clients send `If-None-Match`; **304** skips the body when unchanged. `Cache-Control` is **public** for explorer/news/rates-style data and **private** for authenticated profile/portfolio news. |

---

## Behavior notes

- **413** `payload_too_large` when `Content-Length` exceeds the limit or the body is truncated by `MaxBytesReader`.
- **400** `json_too_deep` or `invalid_json` when JSON is too deeply nested or malformed during depth validation.
- **Non-JSON** bodies (e.g. `application/x-www-form-urlencoded`) are only subject to the **byte** limit via `http.MaxBytesReader`.
- **GET** requests are not affected by body limits (no body expected).
- Existing **export** package constants (`MaxBlockRange`, `MaxTxBlockRange`, default row counts) remain in [`internal/export/meta.go`](../internal/export/meta.go); env vars add an optional **stricter** ceiling on `limit`.

---

## Tests

- [`internal/apiutil/json_depth_test.go`](../internal/apiutil/json_depth_test.go)
- [`internal/server/request_limits_test.go`](../internal/server/request_limits_test.go)
- [`internal/config/config_test.go`](../internal/config/config_test.go) (`TestValidate_ExportCSVRowCaps`, `TestValidate_MaxRequestBodyBytes`)
