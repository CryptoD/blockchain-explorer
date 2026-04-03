# Security HTTP response headers

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **30**. It records which headers the app sets, how they align with common [OWASP](https://owasp.org/) guidance, and what remains to tighten.

**Implementation:** [`internal/server/security_headers.go`](../internal/server/security_headers.go) (`securityHeadersMiddleware`), registered early in [`Run`](../internal/server/run.go).

---

## Headers set on every response

| Header | Value (summary) | OWASP / practice notes |
|--------|------------------|-------------------------|
| **Content-Security-Policy (CSP)** | `default-src 'self'`; `script-src` allows `'self'`, `'unsafe-inline'`, `https://cdn.jsdelivr.net`; `style-src` `'self'` + `'unsafe-inline'`; `img-src` `'self' data: https:`; `connect-src 'self'`; `frame-ancestors 'none'`; `base-uri 'self'`; `form-action 'self'`; `object-src 'none'` | Reduces XSS impact and clickjacking (`frame-ancestors`). **`unsafe-inline` for script/style** weakens XSS defenses; roadmap task **39** may add nonces or hashes. |
| **X-Content-Type-Options** | `nosniff` | Prevents MIME-type confusion ([OWASP Cheat Sheet: HTTP Headers](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html)). |
| **X-Frame-Options** | `DENY` | Legacy complement to `frame-ancestors`; blocks embedding in frames. |
| **Referrer-Policy** | `strict-origin-when-cross-origin` | Limits referrer leakage on cross-origin navigations while keeping same-origin and HTTPS→HTTPS usefulness. |
| **Permissions-Policy** | Disables accelerometer, camera, geolocation, gyroscope, magnetometer, microphone, payment, USB by default | Reduces attack surface for features the app does not use; expand only if needed. |
| **Strict-Transport-Security (HSTS)** | Optional; see below | Tells browsers to use HTTPS only for the site; **only** when TLS is in use. |

---

## HSTS (behind TLS)

HSTS is **not** sent unless:

1. `HSTS_MAX_AGE_SECONDS` is **greater than 0** (default **0** = disabled), and  
2. The request is treated as HTTPS: direct TLS (`Request.TLS`) **or** `X-Forwarded-Proto: https` (typical behind a reverse proxy).

| Environment variable | Meaning |
|---------------------|---------|
| `HSTS_MAX_AGE_SECONDS` | Max-age in seconds (e.g. `31536000` for one year). Valid range: `0`–`63072000` (two years). `0` disables HSTS. |
| `HSTS_INCLUDE_SUBDOMAINS` | If `true`, adds `includeSubDomains` to the header (only enable if **all** subdomains are HTTPS-ready). |

**Recommendation:** Enable HSTS in production **after** HTTPS is verified end-to-end (including between proxy and app if applicable). Mis-issued HSTS with long max-age can lock users out until it expires.

---

## Optional headers not set here

- **Clear-Site-Data**, **Cross-Origin-Opener-Policy**, **Cross-Origin-Resource-Policy**, **Cross-Origin-Embedder-Policy** — add when product requirements justify stricter isolation or logout semantics.
- **Expect-CT** — deprecated; do not use.
- **Public-Key-Pins (HPKP)** — deprecated; do not use.

Re-audit periodically against the [OWASP HTTP Headers Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html) and your deployment topology.
