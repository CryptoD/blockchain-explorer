# Roadmap to 100/100

This document continues [ROADMAP.md](ROADMAP.md). It lists **100 concrete tasks** (with short explanations) aimed at evolving the codebase from a strong production-capable baseline toward **world-class** quality: maintainability at scale, operational excellence, security, performance, and competitive parity with top public explorers.

**Baseline context:** an internal review placed the project at **~72/100**—solid and launchable, with the main gaps in monolithic structure, test depth vs. code volume, and unproven “internet scale” behavior. These tasks are **not** all mandatory for every deployment; prioritize by your threat model and traffic.

**How to use:** work top-to-bottom within each phase where dependencies exist, or pick tasks by theme. Check boxes as you complete items.

---

## Phase 11 — Architecture & modularity

- [ ] **1. Define bounded contexts** — Document domains (auth, explorer, portfolio, watchlist, news, alerts, admin) and their dependencies; avoid new cross-domain leaks.
- [ ] **2. Extract `cmd/server` main** — Keep `main` as thin wiring: parse env, build deps, start server; move logic out of a single huge file.
- [ ] **3. Split `main.go` by domain** — Move handlers into `internal/api/` or `internal/handlers/{auth,explorer,...}` so no file exceeds a maintainable size (e.g. &lt;800 lines).
- [ ] **4. Introduce service interfaces** — For each domain, define `Service` interfaces implemented by structs; handlers depend on interfaces for testing.
- [ ] **5. Centralize route registration** — One function per domain registers routes on `gin.Engine` or `gin.RouterGroup` to avoid a single mega `setupRoutes`.
- [ ] **6. Repository layer for Redis** — Abstract Redis keys and serialization behind `Repository` types (portfolio, watchlist, session, etc.).
- [ ] **7. Config struct validation** — Single `Validate()` on load: fail fast on invalid combinations (e.g. production without admin password).
- [ ] **8. Dependency injection container (light)** — Constructor functions take interfaces; avoid new global mutable singletons beyond unavoidable legacy.
- [ ] **9. Extract pricing to a dedicated package** — Clear boundaries: HTTP client, caching, fiat conversion; mock at interface.
- [ ] **10. Extract blockchain RPC client usage** — All `callBlockchain`-style usage behind one `Blockchain` interface with timeouts and metrics.
- [ ] **11. Email sending behind interface** — `EmailSender` interface; SMTP as one implementation; no-op for tests.
- [ ] **12. News service as standalone module** — Already partially there; ensure no `main` imports for business rules.
- [ ] **13. PDF/CSV export in `internal/export`** — Isolate gofpdf and CSV building from HTTP handlers.
- [ ] **14. Consistent error types** — Domain errors map to stable API codes; avoid stringly-typed errors in hot paths.
- [ ] **15. Version internal APIs** — Prefer `internal/...` packages; document what is stable for future extraction.

## Phase 12 — Testing & quality

- [ ] **16. Set coverage targets per package** — e.g. `internal/news` ≥60%, handlers ≥40%; enforce in CI with gradual ratchet.
- [ ] **17. Handler tests with `httptest` + mocks** — Every exported route group has at least smoke + one error path test.
- [ ] **18. Table-driven tests for validation** — Password policy, pagination, portfolio/watchlist validation in compact tables.
- [ ] **19. Golden files for JSON responses** — Stable snapshots for critical list/detail payloads (optional but high value).
- [ ] **20. Redis integration tests** — Use miniredis or testcontainers for tests that need real Redis semantics beyond mocks.
- [ ] **21. Contract tests for external APIs** — Record/replay or VCR-style for CoinGecko/news when feasible; strict timeouts.
- [ ] **22. Load test script** — k6 or `vegeta` for `/api/search`, auth, and one heavy dashboard path; store baseline numbers.
- [ ] **23. Fuzz tests** — `go test -fuzz` on parsers: txid, addresses, query params.
- [ ] **24. Race detector in CI** — `go test -race` on a schedule or on main; fix races found.
- [ ] **25. Benchmark hot paths** — `Benchmark` for search, cache key build, JSON marshal of large lists.
- [ ] **26. E2E tests (Playwright)** — Already have playwright dep; add CI job for critical user flows (login, search, portfolio).
- [ ] **27. Mutation testing (optional)** — `go-mutesting` or similar on pure packages to find weak tests.
- [ ] **28. Chaos tests (optional)** — toxiproxy or similar for Redis/network flakiness in staging.

## Phase 13 — Security

- [ ] **29. Threat model document** — STRIDE-lite: who attacks, what assets, what mitigations exist.
- [ ] **30. Security headers audit** — Verify CSP, HSTS (behind TLS), X-Frame-Options, etc. against latest OWASP recommendations.
- [ ] **31. CSRF token rotation policy** — Document and test session expiry + CSRF invalidation on password change.
- [ ] **32. Rate limit bypass review** — Ensure metrics and health endpoints cannot be abused for DoS; document exemptions.
- [ ] **33. Input size limits everywhere** — Max body size, max JSON depth, max CSV rows for export.
- [ ] **34. SQL injection N/A audit** — Confirm no string concatenation if SQL is ever introduced; document Redis injection safety.
- [ ] **35. Secrets scanning in CI** — gitleaks or trufflehog on push; block obvious API keys.
- [ ] **36. Dependency update policy** — Renovate/Dependabot + monthly review; Trivy gates stay green.
- [ ] **37. SMTP TLS hardening** — Document production must use verified TLS; `SkipVerify` only in dev with loud logs.
- [ ] **38. Session fixation** — Regenerate session ID on privilege change (login elevation).
- [ ] **39. Content Security Policy for inline scripts** — Reduce inline JS or use nonces where possible.
- [ ] **40. Penetration test (external)** — Annual or before major launch; track findings in issues.

## Phase 14 — Performance & scale

- [ ] **41. Profile production-like load** — CPU/memory profiles under k6; fix top 3 alloc hotspots.
- [ ] **42. Connection pool tuning** — Redis and HTTP client `MaxConns`, timeouts aligned with SLAs.
- [ ] **43. Response compression** — gzip/brotli for JSON/HTML where beneficial; measure CPU tradeoff.
- [ ] **44. Cache stampede protection** — Singleflight or short-lived locks for hot keys (network status, price).
- [ ] **45. Pagination caps enforced server-side** — Already partially; audit all list endpoints for max `page_size`.
- [ ] **46. ETag / conditional GET everywhere appropriate** — Reduce bandwidth for static-ish API responses.
- [ ] **47. Background job batching** — Price alerts: batch Redis reads, spread work across ticks.
- [ ] **48. Read replicas (future)** — If Redis becomes bottleneck, document Redis Cluster or read scaling story.
- [ ] **49. CDN for static assets** — Versioned assets, long cache; separate deploy of front static files.
- [ ] **50. Database migration path** — If moving users off Redis-only: design Postgres schema sketch (even if not implemented).
- [ ] **51. Horizontal scaling checklist** — Session stickiness vs. shared Redis; document required env for N replicas.
- [ ] **52. Graceful shutdown** — Drain HTTP, flush Sentry, close Redis with timeout (expand beyond current if needed).

## Phase 15 — Reliability & resilience

- [ ] **53. SLO definitions** — e.g. 99% of search &lt;500ms; document error budget.
- [ ] **54. Health vs. readiness** — `/health` liveness; `/ready` checks Redis and critical deps (optional separate).
- [ ] **55. Circuit breakers for outbound HTTP** — Per-host breakers for pricing, news, RPC with half-open recovery.
- [ ] **56. Retry budgets** — Cap total retries per request across layers; avoid retry storms.
- [ ] **57. Idempotency keys** — For exports or payments (if ever): safe replays.
- [ ] **58. Dead letter for email queue** — If queue full persists: metrics + admin visibility.
- [ ] **59. Backup/restore runbook** — Redis RDB/AOF strategy; what user data is lost on total loss.
- [ ] **60. Disaster recovery drill** — Quarterly simulated Redis wipe + restore from backup.
- [ ] **61. Feature flags** — Toggle risky features (news, alerts) without redeploy (env or Redis flag).
- [ ] **62. Degraded mode UX** — When Redis down: explicit UI messages; ROADMAP already notes HTML vs JSON rate limit—**fix** that gap.
- [ ] **63. Queue depth alerts** — Prometheus alerts for email queue, background job backlog.

## Phase 16 — API & developer experience

- [ ] **64. OpenAPI 3 spec** — Generate or hand-maintain `openapi.yaml` for `/api/v1`; validate in CI.
- [ ] **65. API versioning policy** — Document deprecation: minimum notice, sunset headers.
- [ ] **66. Consistent error envelope** — Every error JSON includes `code`, `message`, `correlation_id`, `timestamp`.
- [ ] **67. Pagination metadata standard** — Same shape for all list endpoints (`total`, `page`, `page_size`, `has_more`).
- [ ] **68. Public Postman/collection** — Importable collection for partners and testers.
- [ ] **69. Idempotent PUT/PATCH semantics** — Document ETags or version fields for portfolios where relevant.
- [ ] **70. Webhooks (optional)** — Outbound events for enterprise: block confirmation, alert triggered.
- [ ] **71. API keys for machine access** — Separate from session cookies for automation (scoped, rotatable).
- [ ] **72. Changelog** — `CHANGELOG.md` with semver for API and app releases.

## Phase 17 — Frontend & UX

- [ ] **73. Bundle analysis** — Split JS, tree-shake, lazy-load heavy charts.
- [ ] **74. Accessibility audit** — WCAG 2.1 AA spot-check: keyboard nav, contrast, aria on autocomplete.
- [ ] **75. i18n completeness** — All user-visible strings through translation maps; RTL if targeted.
- [ ] **76. Mobile responsive audit** — Real devices for dashboard and explorer flows.
- [ ] **77. Offline / slow network** — Skeleton loaders; retry on failed fetches for news/prices.
- [ ] **78. Progressive enhancement** — Core search works without full JS where possible (optional).
- [ ] **79. Error boundaries (frontend)** — User-friendly error component vs raw JSON in face of 502.
- [ ] **80. Performance budgets** — LCP/CLS targets; track in Lighthouse CI (optional).

## Phase 18 — Operations & DevOps

- [ ] **81. Helm chart or compose prod overlay** — Production-ready env template with resource limits.
- [ ] **82. Multi-stage Docker hardening** — Distroless or minimal runtime; non-root; read-only root fs where possible.
- [ ] **83. Image signing** — cosign for released images.
- [ ] **84. GitHub Actions OIDC deploy** — No long-lived cloud keys in secrets where avoidable.
- [ ] **85. Staging environment** — Parity with prod config; anonymized data.
- [ ] **86. Log aggregation** — Ship JSON logs to Loki/ELK; retention policy.
- [ ] **87. Cost monitoring** — Alerts on egress or Redis memory growth anomalies.
- [ ] **88. On-call runbook** — PagerDuty/Opsgenie playbook: “Redis full”, “RPC down”, “Sentry spike”.

## Phase 19 — Documentation & compliance

- [ ] **89. Architecture Decision Records** — `docs/adr/` for major choices (Redis-only, session model).
- [ ] **90. CONTRIBUTING.md** — PR checklist, local dev, security disclosure pointer.
- [ ] **91. DATA_PROCESSING.md** — GDPR-style: what PII stored, retention, deletion path.
- [ ] **92. LICENSE and third-party notices** — `NOTICE` file for bundled assets/fonts if any.
- [ ] **93. Operator manual** — One PDF or doc: install, upgrade, backup, rollback.
- [ ] **94. API deprecation communications** — Template for email/changelog when breaking changes loom.
- [ ] **95. Security.txt** — `/.well-known/security.txt` pointing to [SECURITY.md](SECURITY.md).

## Phase 20 — Product & competitive parity

- [ ] **96. Real provider credentials path** — Complete Phase 10 roadmap item: production RPC and pricing keys with key rotation doc.
- [ ] **97. Feature parity matrix** — Table vs 2–3 named competitors: blocks, mempool, fees, RBF, Lightning (honest gaps).
- [ ] **98. Public status page** — Uptime history for API and explorer (external service or self-hosted).
- [ ] **99. Community channel** — Discussions, Discord, or forum linked from README for feedback loop.
- [ ] **100. Release certification checklist** — Pre-1.0 or GA: security review, load test, DR drill, legal review of ToS/Privacy.

---

*This list is ambitious by design. Reaching “100” in a qualitative sense is a moving target as the industry improves—treat it as a **north star**, not a single sprint.*
