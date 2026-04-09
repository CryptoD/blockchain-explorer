# Development Roadmap

## Phase 1 — Core explorer & API
- [x] **DONE** — Search (blocks, transactions, addresses)
- [x] **DONE** — REST API (versioned and legacy)
- [x] **DONE** — Redis caching and session storage
- [x] **DONE** — Frontend (index, bitcoin detail, symbols)

## Phase 2 — Authentication & users
- [x] **DONE** — Session-based auth (login, logout, register)
- [x] **DONE** — CSRF protection for state-changing requests
- [x] **DONE** — Admin and user roles (RBAC)
- [x] **DONE** — Auth middleware and protected routes

## Phase 3 — Portfolios
- [x] **DONE** — Portfolio data model and Redis storage
- [x] **DONE** — Portfolio CRUD and valuation (fiat, pricing)
- [x] **DONE** — Portfolio export (CSV, PDF, JSON)
- [x] **DONE** — Dashboard with portfolio metrics and charts

## Phase 4 — Rate limiting & security
- [x] **DONE** — Rate limiting (IP and per-user, Redis-backed)
- [x] **DONE** — Export rate limits and logging
- [x] **DONE** — Security headers and safe defaults

## Phase 5 — User profiles & preferences
- [x] **DONE** — User profile model in Redis (theme, language, notifications, default landing page)
- [x] **DONE** — Authenticated profile GET/PATCH with validation
- [x] **DONE** — Profile/settings page (profile UI)
- [x] **DONE** — Theme and language applied immediately and persisted across sessions

## Phase 6 — Watchlists
- [x] **DONE** — Watchlist data model (entries: symbol/address, tags, notes, group)
- [x] **DONE** — Redis storage and CRUD for watchlists and entries
- [x] **DONE** — Quotas (per-user watchlists, per-watchlist entries)
- [x] **DONE** — Watchlist management endpoints (auth, RBAC)
- [x] **DONE** — Add to watchlist from search/detail (explorer UI)
- [x] **DONE** — Dashboard watchlist panel (prices, links, group-by)
- [x] **DONE** — Reorder (up/down) and grouping (asset type, custom group); Group field persisted

## Phase 7 — Financial news & contextual data
- [x] **DONE** — News provider selection + documentation (TheNewsAPI)
- [x] **DONE** — News service with Redis caching (fresh TTL + stale fallback on provider error/rate-limit)
- [x] **DONE** — News endpoints (legacy and versioned): `GET /api/news/:symbol`, `GET /api/news/portfolio/:id`
- [x] **DONE** — Frontend news widgets (dashboard + symbols) with loading/error states + filters
- [x] **DONE** — User preferences for favorite/muted news sources (profile UI + backend filtering)

## Phase 8 — Notifications & alerts
- [x] **DONE** — PriceAlert model + Redis persistence + CRUD endpoints
- [x] **DONE** — Background evaluation job (efficient Redis SCAN, metrics logging, triggered alert handling)
- [x] **DONE** — In-app notifications (Redis-backed) + notification center UI (read/dismiss)
- [x] **DONE** — Email delivery via SMTP (templated welcome/alert/admin-critical) + user opt-in preferences
- [x] **DONE** — Profile settings updated (email field + granular email toggles)

## Phase 9 — Observability & operations *(concluded)*
- [x] **DONE** — Structured logging (`internal/logging`): JSON logs, `LOG_LEVEL`, shared fields (`component`, `event`, safe search query hashing)
- [x] **DONE** — Prometheus-compatible `GET /metrics` (`internal/metrics`): HTTP latency/counts, cache and background-job metrics; optional `METRICS_TOKEN`; rate-limit exemption for scrapers
- [x] **DONE** — Sentry integration (`internal/sentryutil`): environment, release, trace/error sample rates, request/user scope, `BeforeSend` scrubbing of sensitive headers
- [x] **DONE** — Correlation IDs (`internal/correlation`): propagate `X-Correlation-ID` / `X-Request-ID`, response headers and JSON on errors/health; per-run IDs for background jobs
- [x] **DONE** — Monitoring playbook: `monitoring/README.md`, Grafana dashboard JSON, Prometheus alert rules, Alertmanager escalation example, `promtool` check script

## Continuation — path to “100/100” quality

This roadmap is the **historical phase checklist** (Phases 1–9 delivered; Phase 10 is future work below). **[ROADMAP_TO_100.md](ROADMAP_TO_100.md)** is the **continuation**: **100 concrete tasks** with explanations to push the project toward world-class maintainability, security, performance, and competitive parity.

**Status:** In **[ROADMAP_TO_100.md](ROADMAP_TO_100.md)**, tasks **1–61** are marked **complete** (including feature flags for news and price alerts). Tasks **62–100** remain **open**. That file is the live checklist—keep it updated as work lands.

**Context:** an internal code review rated the codebase at **~72/100**—strong and launchable; the main gaps called out were monolith size, test depth vs. code volume, and hardening for extreme scale. Use the new file as the backlog to **perfect** the project beyond the original roadmap’s “done” milestones.

## Phase 10 — Future
- [ ] Replace placeholder RPC/pricing keys with real provider credentials for full data fidelity
- [ ] Make HTML routes resilient to API rate limiting (avoid returning JSON rate-limit responses for pages)
- [ ] Additional watchlist/portfolio enhancements
- [ ] Further UX and performance improvements
