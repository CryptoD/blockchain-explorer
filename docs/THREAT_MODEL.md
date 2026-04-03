# Threat model (STRIDE-lite)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **29**. It is a **lightweight** STRIDE view: enough to reason about who might attack, what matters, and what the codebase already does to reduce risk. It is **not** a formal certification or penetration-test report.

**Scope:** the blockchain explorer **web app and API** (`cmd/server`, `internal/server`, Redis-backed state, optional GetBlock / pricing / news / email integrations). **Out of scope:** hosting provider hardening, OS hardening, client browser malware, physical access to servers.

**Assumptions:** production uses **TLS** in front of the app, **strong** `ADMIN_*` and provider secrets in environment variables, and **network isolation** so Redis is not exposed to the public internet.

---

## 1. Assets (what we protect)

| Asset | Why it matters |
|-------|----------------|
| **User accounts** | Password hashes, roles, profile data in Redis (`user:*`, sessions). |
| **Sessions & cookies** | `session_id` cookie binds browser to server-side session; compromise = account takeover. |
| **CSRF tokens** | Bound to session; protect state-changing HTTP from cross-site requests. |
| **Admin surface** | Cache clear, status, rate-limit views; powerful in production. |
| **Redis contents** | Cache keys, portfolios, watchlists, feedback blobs, rate-limit counters—integrity and confidentiality for multi-tenant data. |
| **Provider secrets** | `GETBLOCK_*`, `THENEWSAPI_*`, `SENTRY_*`, SMTP, metrics token—direct cost and abuse if leaked. |
| **Blockchain / pricing correctness** | Users rely on search results and valuations; tampering or stale cache misleads rather than “steals” a secret. |
| **Availability** | Explorer and auth must remain responsive; abuse of heavy endpoints or Redis can degrade service. |
| **Logs & telemetry** | Correlation IDs, Sentry events—may contain PII or hints for further attack if over-logged. |

---

## 2. Threat actors (who attacks)

| Actor | Motivation | Typical capability |
|-------|------------|-------------------|
| **Anonymous internet** | Scrape, spam, DoS, credential stuffing, exploit public APIs. | No prior credentials; high volume, automated. |
| **Authenticated user** | Access or modify **other** users’ data; abuse exports; escalate to admin if misconfiguration allows. | Valid session; constrained by authz and Redis key layout. |
| **Admin (or stolen admin session)** | Full use of admin routes; clear caches; observe operational data. | Strong if `ADMIN_*` weak or session hijacked. |
| **Operator / misconfiguration** | Accidentally expose Redis, disable TLS, ship dev defaults. | Environment and deployment mistakes. |
| **Dependency / supply chain** | Backdoored module or base image. | Mitigated by CI scans and pinning; residual risk remains. |

We do **not** model nation-state APTs in detail; many controls below still help against opportunistic and script-kiddie threats.

---

## 3. STRIDE-lite matrix

STRIDE categories: **S**poofing, **T**ampering, **R**epudiation, **I**nformation disclosure, **D**enial of service, **E**levation of privilege.

Each row: risk to an asset, relevant mitigations **as implemented or configured** in this repo (not every line of defense is listed).

| Category | Example threat | Primary assets | Mitigations (indicative) |
|----------|----------------|----------------|---------------------------|
| **S** | Attacker forges session or impersonates another user | Sessions, accounts | Server-side sessions in Redis; cookie-based `session_id`; password verification on login; **CSRF** on state-changing and admin routes when session present ([`csrfMiddleware`](../internal/server/updatepricealerthandler.go)); optional **Secure** cookie flag via config ([`SecureCookies`](../internal/config/config.go)). |
| **S** | Caller pretends to be admin | Admin APIs | **Role** checks and admin routes behind auth; CSRF for admin as applicable; production requires strong `ADMIN_*` ([`Validate`](../internal/config/config.go)). |
| **T** | Modify another user’s portfolio or profile | Redis user data | Keys scoped by **username** / id (`portfolio:{user}:{id}`); handlers use authenticated **username** from session ([`authMiddleware`](../internal/server/updateprofilehandler.go)). |
| **T** | Tamper with cached blockchain JSON | Cache integrity | Redis over trusted network; app does not trust client for canonical chain data—**server** fetches RPC/cached values. |
| **R** | User denies submitting feedback or admin action | Audit | Structured logging with components and correlation IDs ([`internal/logging`](../internal/logging/)); not a full legal audit trail—**gap** if non-repudiation is required. |
| **I** | Leak PII, session tokens, or stack traces to clients | Profiles, errors | Typed errors and stable API codes ([`internal/apperrors`](../internal/apperrors/)); avoid returning raw `err.Error()` for 5xx in hot paths; Sentry sampling configurable. |
| **I** | Leak metrics or Redis contents | Ops data | **Metrics** optional auth token ([`METRICS_TOKEN`](../internal/config/config.go)); admin routes require authenticated admin. |
| **I** | Exfiltrate provider API keys from repo or runtime | Secrets | Env-based config; **do not** commit secrets; CI can add secret scanning (see roadmap task 35). |
| **D** | Flood `/api/search`, login, or exports | Availability | **Rate limiting** per IP and per user ([`rateLimitMiddleware`](../internal/server/updateprofilehandler.go)); stricter **export** limits; optional Redis failure → in-memory limiter fallback (degraded but bounded). |
| **D** | Exhaust Redis or upstream RPC | Backend | Timeouts on blockchain/pricing HTTP clients; cache TTLs; pagination caps ([`internal/apiutil`](../internal/apiutil/pagination.go)). |
| **E** | User becomes admin | Roles | Separate **admin** user and role checks; default dev admin only in development ([`initializeDefaultAdmin`](../internal/server/init.go)); production **refuses start** without admin password ([`Validate`](../internal/config/config.go)). |
| **E** | CSRF bypass on POST/PUT/DELETE | Sessions | CSRF token stored server-side; **X-CSRF-Token** required for state-changing routes with session cookie. |

---

## 4. Residual risks & follow-ons

- **Headers (CSP, HSTS, framing)** — covered by roadmap task **30**; not fully specified here.
- **Session fixation / rotation on login** — roadmap task **38**.
- **Input limits** on large bodies/exports — roadmap task **33**.
- **Redis exposed to the internet** — defeats many assumptions; **must** be private to the app VPC/network.
- **Default dev credentials** — documented in README; **never** use in production.

---

## 5. Maintaining this document

Update this file when you add a **new externally reachable surface** (OAuth, webhooks, new admin powers) or change **trust boundaries** (e.g. Redis split, multi-region). Link major mitigations to code or config paths as above.
