# Postman collection (`/api/v1`)

Import **Postman Collection v2.1** for partners and testers:

- File: [`blockchain-explorer.postman_collection.json`](blockchain-explorer.postman_collection.json)

**In Postman:** **Import → File**, select the JSON above. Adjust the collection variable **`baseUrl`** (default `http://localhost:8080`) if your server differs.

**Authenticated flows**

1. Set **`username`** and **`password`** (collection variables).
2. Send **Auth → Login** first. The request **Tests** script stores **`csrfToken`** from the JSON body; Postman keeps the **`session_id`** cookie for later calls.
3. For routes that need path ids, copy values from list responses into **`portfolioId`**, **`alertId`**, **`notificationId`**, **`watchlistId`**, **`watchlistEntryIndex`**, or **`apiKeyPublicId`** as needed.

**Machine API keys (`Bearer bkx_`)**

1. Set **`apiKeyBearer`** to a token returned from **User — API keys → Create user API key** (or from the admin UI / `POST /api/v1/user/api-keys` outside Postman).
2. Use the **User — API keys (Bearer)** and **Admin** folder entries that send only `Authorization` (no cookie, no CSRF). Service keys need `admin:read` / `admin:write` scopes as in [docs/API_KEYS.md](../docs/API_KEYS.md).

**CSRF**

- After login, mutating methods (`POST`, `PUT`, `PATCH`, `DELETE`) on user and feedback routes send **`X-CSRF-Token: {{csrfToken}}`**.
- **All** `GET` (and other methods) under **`/api/v1/admin/*`** require the same header when using a session.
- **Login** and **Register** do not use CSRF (by design).

**Regenerating the file**

When routes change, run from the repo root:

```bash
python3 scripts/gen-postman-collection.py > postman/blockchain-explorer.postman_collection.json
```

The machine-readable contract remains **[`openapi.yaml`](../openapi.yaml)** at the repository root.
