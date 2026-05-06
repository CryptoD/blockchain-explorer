# Machine API keys (Bearer `bkx_*`)

This document supports [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **71**. It describes how **automated clients** authenticate with **`Authorization: Bearer <token>`** instead of the browser **`session_id`** cookie.

**OpenAPI:** [openapi.yaml](../openapi.yaml) security scheme `apiKeyBearer`.

---

## Token format

Plaintext tokens look like:

```text
bkx_<16 lowercase hex chars>_<secret>
```

- **`bkx_`** — fixed prefix so the server can route authentication without guessing.
- **16 hex characters** — public id (key id) for listing and revocation.
- **`secret`** — high-entropy segment (base64url), never stored in clear text.

The server persists only a **SHA-256 hash** of the full plaintext string in Redis.

---

## User-owned keys

### Scopes

| Scope        | Meaning |
|-------------|---------|
| `user:read` | `GET`/`HEAD`/`OPTIONS` on `/api/v1/user/*` routes (and portfolio news below). |
| `user:write` | Mutating methods on those routes. Implies read for API-key auth. |

Create a key:

- `POST /api/v1/user/api-keys`  
  Body: `{"name":"CI","scopes":["user:read","user:write"]}`  
  When using a **browser session**, send `X-CSRF-Token` as today.

List / revoke:

- `GET /api/v1/user/api-keys`
- `DELETE /api/v1/user/api-keys/{id}`

**Quota:** configurable via `API_KEYS_MAX_PER_USER` (default **10** active keys).

---

## Service keys (admin-managed)

For partners and internal jobs that need **`/api/v1/admin/*`** without an interactive session:

| Scope          | Meaning |
|----------------|---------|
| `admin:read`   | e.g. `GET /api/v1/admin/status`, `GET /api/v1/admin/cache?action=stats` |
| `admin:write`  | Above plus `GET /api/v1/admin/cache?action=clear`. Implies read for API-key auth. |

- `GET /api/v1/admin/api-keys` — list service key metadata (Bearer with `admin:read`, or admin session).
- `POST /api/v1/admin/api-keys` — **browser session only** (CSRF + admin role). API keys cannot mint or rotate other API keys.
- `DELETE /api/v1/admin/api-keys/{id}` — **browser session only** (same reason).

Service keys are denied on `/api/v1/user/*` and authenticated portfolio news routes.

---

## CSRF

Global `csrfMiddleware` requires `X-CSRF-Token` when a **`session_id` cookie** is present on state-changing routes.  

If you send **`Authorization: Bearer bkx_...` only** (no session cookie), CSRF is **not** applied to that request. **Do not** attach your browser session cookie when using API keys from scripts; mixed cookie + bearer can still trigger CSRF checks.

---

## Disabling keys

- Set `API_KEYS_ENABLED=false` / `0` to reject bearer authentication (see `.env.example`).

---

## Related

- Postman: [postman/README.md](../postman/README.md).
- API deprecation policy: [API_VERSIONING.md](API_VERSIONING.md).
- REST write semantics (portfolios/watchlists): [REST_WRITE_SEMANTICS.md](REST_WRITE_SEMANTICS.md).
