# CSRF tokens and sessions

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **31**. It describes how **sessions**, **CSRF tokens**, and **password change** interact, and where the behavior is implemented.

For **multiple app replicas** behind a load balancer, see [HORIZONTAL_SCALING.md](HORIZONTAL_SCALING.md) (task **51**): shared Redis allows **non-sticky** sessions when Redis is healthy.

---

## Session lifecycle

| Event | Behavior |
|-------|----------|
| **Login** | Any previous **`session_id`** cookie is **destroyed** first ([`loginHandler`](../internal/server/updatepricealerthandler.go)) to mitigate **session fixation** and stale sessions when switching accounts. Then a **new** session id is created ([`createSession`](../internal/server/init.go)); it is stored in **Redis** with TTL **24 hours** and mirrored in an in-process map for resilience. The HTTP **Set-Cookie** uses `Max-Age` **86400** (24h) and [`SecureCookies`](../internal/config/config.go) when configured. The login JSON response includes **`csrfToken`**. |
| **Logout** | [`destroySession`](../internal/server/init.go) removes the session and CSRF entries from memory and Redis ([`SessionRepo.DeleteSession`](../internal/repos/session.go)). |
| **Validation** | [`validateSession`](../internal/server/init.go) reads the **`session_id`** cookie. When Redis is configured, **Redis is authoritative**: if the session key is missing or expired (`redis.Nil`), the session is **invalid** and the in-memory copy for that id is cleared—state-changing requests must not rely on stale memory after Redis TTL. If Redis is temporarily unavailable, validation may fall back to the in-memory store. |

Session ids are generated with **`base64.RawURLEncoding`** (no `=` padding) to avoid cookie/header edge cases with padded base64.

---

## CSRF policy

| Topic | Policy |
|-------|--------|
| **Storage** | CSRF token is keyed by **session id** in Redis ([`SessionRepo.SetCSRF`](../internal/repos/session.go), TTL **24 hours**) and in memory when Redis is unavailable. |
| **When required** | [`csrfMiddleware`](../internal/server/updatepricealerthandler.go) requires header **`X-CSRF-Token`** on **state-changing** methods (POST, PUT, PATCH, DELETE) and on **admin** routes when a **`session_id`** cookie is present. Login/register are exempt. |
| **Rotation on password change** | **`PATCH /api/v1/user/password`** ([`changePasswordHandler`](../internal/server/updateprofilehandler.go)) verifies **`current_password`**, updates the password hash, then calls **`CreateOrUpdateCSRFToken`** for the **current** session. The JSON response includes a new **`csrfToken`**. Any previously issued CSRF value for that session **stops working** immediately. Clients must send the **new** token on subsequent mutating requests. |

---

## API: change password

- **Route:** `PATCH /api/v1/user/password` (authenticated; CSRF required like other PATCH routes).
- **Body:** `{ "current_password": "<string>", "new_password": "<string>" }`
- **Success:** `200` with `{ "message": "Password updated", "csrfToken": "<new token>" }`
- **New password** must satisfy [`isStrongPassword`](../internal/server/init.go) (same rules as registration).
- **Wrong current password:** `401` with `invalid_credentials`.

---

## Tests

Coverage lives in [`internal/server/auth_test.go`](../internal/server/auth_test.go):

- **`TestPasswordChange_RotatesCSRF`** — after a successful password change, the old CSRF is rejected for `PATCH /profile`; the new token works; login with the new password succeeds.
- **`TestPasswordChange_WrongCurrentPassword`** — incorrect current password returns **401**.
- **`TestSession_InvalidWhenRedisSessionKeyMissing`** — if the Redis session key is removed (same **`redis.Nil`** path as TTL expiry), **`GET /profile`** returns **401**.
- **`TestSession_InvalidAfterServerSideDestroy`** — server-side session teardown invalidates the browser session.
- **`TestLogin_DestroyPriorSessionOnElevation`** — logging in as another user while sending the first user’s session cookie invalidates the first session; the old cookie no longer works for `/profile`.

---

## Related roadmap items

- **38** — Session fixation: prior session invalidated on successful login ([`loginHandler`](../internal/server/updatepricealerthandler.go)); see tests in [`auth_test.go`](../internal/server/auth_test.go).
- **29** — [Threat model](THREAT_MODEL.md) (CSRF and sessions in the STRIDE matrix).
