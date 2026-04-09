# Degraded mode UX (Redis down and rate limits)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **62**.

## Readiness banner (Redis or dependencies down)

When **`GET /ready`** returns **503** or JSON with `status` ≠ **`ready`**, the app is **not** ready to serve traffic that depends on Redis (sessions, portfolios, rate limits backed by Redis, etc.).

**UI:** Main HTML shells ([`index.html`](../index.html), [`dashboard.html`](../dashboard.html), [`profile.html`](../profile.html), [`symbols.html`](../symbols.html), [`bitcoin.html`](../bitcoin.html), [`admin.html`](../admin.html)) load [`static/js/degraded-mode.js`](../static/js/degraded-mode.js), which:

1. Fetches **`GET /ready`** with `Accept: application/json` (exempt from the global rate limit; see [RATE_LIMITS.md](RATE_LIMITS.md)).
2. If the response is not OK or `status` is not `ready`, shows a **dismissible amber banner** at the top with the server `error` string when present.
3. Re-checks every **60 seconds** and removes the banner if readiness recovers.

Orchestrators typically stop routing to pods that fail readiness; this banner still helps **single-node** or **direct browser** access during incidents and local development when Redis is stopped.

## Rate limit: HTML vs JSON (429)

The global rate limiter returns **JSON** for API routes (`/api/...`) and for static assets under `/static/`, `/dist/`, `/images/` so clients and CDNs see a consistent machine-readable body.

For **browser navigations** to HTML app shells (`/`, `/dashboard`, `/profile`, `/symbols`, `/admin`, `/bitcoin`, `*.html`, or other GETs with `Accept: text/html`), a **429 Too Many Requests** returns a **small HTML page** explaining the limit instead of a JSON error—so users are not stuck viewing raw API JSON.

Implementation: [`wantsHTMLRateLimitPage`](../internal/server/rate_limit_html.go), [`rateLimitErrorResponse`](../internal/server/rate_limit_html.go) in [`rateLimitMiddleware`](../internal/server/updateprofilehandler.go).

## Related

- [HEALTH_AND_READINESS.md](HEALTH_AND_READINESS.md) — liveness vs readiness.
- [RATE_LIMITS.md](RATE_LIMITS.md) — limits and exemptions.
