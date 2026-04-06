# CDN and static assets (versioned URLs, long cache)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **49**.

## Goals

- **Versioned asset URLs** so browsers and CDNs can cache **`/dist`**, **`/static`**, and **`/images`** aggressively without serving stale JS/CSS after a release.
- **Optional long `Cache-Control`** on the origin when those URLs carry a version (query `?v=` or immutable filenames).
- **Separate deploy** of static files to object storage + CDN (S3 + CloudFront, GCS + Cloud CDN, Azure Blob + Front Door, etc.) while the Go app serves HTML and API, or a split where HTML also comes from CDN with API-only on the app.

## Build pipeline (`npm run build`)

1. **`npm run build:css`** — produces `dist/styles.css` (Tailwind + theme).
2. **`npm run stamp:assets`** — runs [`scripts/stamp-frontend-assets.cjs`](../scripts/stamp-frontend-assets.cjs):
   - Writes **`build/stamped/*.html`** (source `*.html` in the repo root is **not** modified).
   - Appends **`?v=<version>`** to every `href` / `src` pointing at **`/dist/`**, **`/static/`**, or **`/images/`**.
   - Writes **`dist/.asset-version`** for operators.
   - **Version resolution:** `ASSET_VERSION` → `GITHUB_SHA` (7 chars) → `git rev-parse --short HEAD` → SHA256 prefix of `dist/styles.css` → `dev`.

### Optional CDN prefix at build time

Set **`CDN_BASE_URL`** (no trailing slash) when stamping, e.g. `https://d111111abcdef8.cloudfront.net`. Stamped HTML will reference absolute URLs like `https://…/dist/styles.css?v=abc123`.

**Runtime:** set **`CDN_BASE_URL`** to the **same** origin string on the server so [Content-Security-Policy](../internal/server/security_headers.go) allows scripts, styles, and images from that host.

## Server behavior

- If **`build/stamped/index.html`** exists (after `npm run build`), the app serves HTML from **`build/stamped/`**; otherwise it serves unstamped files from the working directory (local dev, E2E with `build:css` only).
- **`STATIC_ASSET_CACHE_MAX_AGE_SECONDS`** (default **0**): when **> 0**, responses under **`/static/`**, **`/dist/`**, and **`/images/`** get  
  `Cache-Control: public, max-age=<seconds>, immutable`.  
  **Use only with versioned URLs** (stamped HTML or equivalent), or clients may cache unversioned paths for a year.
- **`CDN_BASE_URL`**: optional; must be a **scheme + host** only (no path). Used for CSP only; it does not rewrite responses.

## Docker image

The [Dockerfile](../Dockerfile) uses a **Node** stage to run `npm ci`, `build:css`, and the stamp script, then copies **`build/stamped/`** into the runtime image as the shell HTML files. **`dist/`** comes from the same stage. **`static/`** and **`images/`** still come from the repository (builder context).

Pass a version into the image build when Git metadata is unavailable:

```bash
docker build --build-arg ASSET_VERSION="$(git rev-parse --short HEAD)" -t blockchain-explorer:latest .
```

## Separate static deploy (typical CloudFront + S3)

1. **Build** with the same **`ASSET_VERSION`** and **`CDN_BASE_URL`** you will use in production.
2. **Upload** `dist/`, `static/`, and `images/` to the bucket backing the CDN (preserve paths: `dist/styles.css`, `static/js/…`, `images/…`).
3. **Cache behavior** on the CDN: long TTL for those paths; optionally match `?v=*` or path patterns.
4. **Invalidate** only when needed; with versioned query strings, **new HTML** (short TTL or invalidation) points at new `?v=` and old objects can expire naturally.
5. **Origin** for the Go app: either still serves `/static` etc. as fallback, or returns **404** for those paths if everything is on CDN (ensure stamped HTML never references the origin for assets in that mode).

## Related configuration

| Variable | Purpose |
|----------|---------|
| `ASSET_VERSION` | Override stamp version (build / Docker ARG). |
| `CDN_BASE_URL` | Build: absolute asset URLs. Runtime: CSP allowlist. |
| `STATIC_ASSET_CACHE_MAX_AGE_SECONDS` | e.g. `31536000` (1 year) in production behind versioned URLs. |

See also [SECURITY_HEADERS.md](SECURITY_HEADERS.md) (CSP) and [INPUT_LIMITS.md](INPUT_LIMITS.md).
