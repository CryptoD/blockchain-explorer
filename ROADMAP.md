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

## Phase 7 — Future
- [ ] Price alerts (notifications)
- [ ] Additional watchlist/portfolio enhancements
- [ ] Further UX and performance improvements
