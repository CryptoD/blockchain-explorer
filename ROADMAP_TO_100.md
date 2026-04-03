# Roadmap to 100/100

This document continues [ROADMAP.md](ROADMAP.md). It lists **100 concrete tasks** (with short explanations) aimed at evolving the codebase from a strong production-capable baseline toward **world-class** quality: maintainability at scale, operational excellence, security, performance, and competitive parity with top public explorers.

**Baseline context:** an internal review placed the project at **~72/100**—solid and launchable, with the main gaps in monolithic structure, test depth vs. code volume, and unproven “internet scale” behavior. These tasks are **not** all mandatory for every deployment; prioritize by your threat model and traffic.

**How to use:** work top-to-bottom within each phase where dependencies exist, or pick tasks by theme. Check boxes as you complete items.

---

## Phase 11 — Architecture & modularity

- [x] **1. Define bounded contexts** — Document domains (auth, explorer, portfolio, watchlist, news, alerts, admin) and their dependencies; avoid new cross-domain leaks. **Done:** [docs/BOUNDED_CONTEXTS.md](docs/BOUNDED_CONTEXTS.md).
- [x] **2. Extract `cmd/server` main** — Keep `main` as thin wiring: parse env, build deps, start server; move logic out of a single huge file. **Done:** [`cmd/server/main.go`](cmd/server/main.go) calls [`internal/server.Run`](internal/server/run.go); handler and app logic live under [`internal/server/`](internal/server/); tests co-located under `internal/server`.
- [x] **3. Split monolithic server package by domain** — **Done:** logic is split across multiple `internal/server/*.go` files (same package to avoid a large wiring refactor); entrypoint [`internal/server/run.go`](internal/server/run.go) (`Run`), plus domain-grouped files such as [`internal/server/init.go`](internal/server/init.go) (i18n, types, sessions), [`internal/server/updateprofilehandler.go`](internal/server/updateprofilehandler.go) (auth, profile, news, alerts), [`internal/server/searchblockchain.go`](internal/server/searchblockchain.go), [`internal/server/explorer_fetch.go`](internal/server/explorer_fetch.go), [`internal/server/getusdperfiat.go`](internal/server/getusdperfiat.go), [`internal/server/collectmetrics.go`](internal/server/collectmetrics.go), etc. **No source file exceeds ~800 lines.** A follow-on step is optional subpackages (`internal/handlers/...`) once interfaces exist (see tasks 4–5).
- [x] **4. Introduce service interfaces** — **Done:** Domain contracts live in [`internal/server/service_interfaces.go`](internal/server/service_interfaces.go) (`ExplorerService`, `AuthService`, `PortfolioService`, `WatchlistService`, `AlertService`, `AdminService`, `FeedbackService`, plus `NewsAppService` / `EmailAppService` wrapping `*news.Service` / `*email.Service`). Defaults in [`internal/server/service_defaults.go`](internal/server/service_defaults.go); [`ResetDefaultServices`](internal/server/service_defaults.go) and `Set*Service` helpers restore or inject mocks for tests. Existing [`pricing.Client`](internal/pricing/) / [`blockchain.RPCClient`](internal/blockchain/) injection via [`SetPricingClient`](internal/server/getusdperfiat.go) / [`SetBlockchainClient`](internal/server/getusdperfiat.go) remains. Example mock test: [`TestExplorerService_MockSearchHandlerUsesInterface`](internal/server/service_test.go).
- [x] **5. Centralize route registration** — **Done:** [`internal/server/routes.go`](internal/server/routes.go) registers routes per domain (`registerExplorerRoutesV1`, `registerFeedbackRoutesV1`, `registerNewsRoutesV1`, `registerAuthRoutesV1`, `registerUserRoutesV1` + `registerUser*Routes` for profile, notifications, alerts, portfolio, watchlist, `registerAdminRoutesV1`), plus `registerStaticRoutes`, `registerHealthAndMetricsRoutes`, and `registerLegacyAPIRoutes`. [`Run`](internal/server/run.go) calls these after dependency wiring.
- [x] **6. Repository layer for Redis** — **Done:** [`internal/repos`](internal/repos/) defines key prefixes/builders ([`keys.go`](internal/repos/keys.go)) and typed repositories: [`PortfolioRepo`](internal/repos/portfolio.go), [`WatchlistRepo`](internal/repos/watchlist.go), [`SessionRepo`](internal/repos/session.go) (session + CSRF), [`UserRepo`](internal/repos/user.go), [`FeedbackRepo`](internal/repos/feedback.go), [`AdminRepo`](internal/repos/admin.go), grouped as [`repos.Stores`](internal/repos/stores.go). [`internal/server/repos_wire.go`](internal/server/repos_wire.go) holds `appRepos`, assigned in [`Run`](internal/server/run.go) and tests. Default domain services and auth/session paths use these repositories instead of ad hoc `rdb` key strings for those domains. Explorer cache keys (`address:`, `tx:`, …) remain in handlers for a later pass if desired.
- [x] **7. Config struct validation** — Single `Validate()` on load: fail fast on invalid combinations (e.g. production without admin password). **Done:** [`internal/config/config.go`](internal/config/config.go) (`Load` calls `Validate`); tests in [`internal/config/config_test.go`](internal/config/config_test.go).
- [x] **8. Dependency injection container (light)** — Constructor functions take interfaces; avoid new global mutable singletons beyond unavoidable legacy. **Done:** [`internal/server/deps.go`](internal/server/deps.go) (`Dependencies`, `NewDependencies`, `applyDependencies` in [`Run`](internal/server/run.go)); [`GetDependencies`](internal/server/deps.go) for introspection; tests in [`internal/server/deps_test.go`](internal/server/deps_test.go). Legacy package globals remain for existing handlers; new wiring should use the struct rather than new globals.
- [x] **9. Extract pricing to a dedicated package** — Clear boundaries: HTTP client, caching, fiat conversion; mock at interface. **Done:** [`internal/pricing`](internal/pricing/) — [`doc.go`](internal/pricing/doc.go) describes layers; [`interfaces.go`](internal/pricing/interfaces.go) (`Client`, `AssetPricer`, fetchers); [`fiat.go`](internal/pricing/fiat.go) / [`fiat_convert.go`](internal/pricing/fiat_convert.go); [`coingecko.go`](internal/pricing/coingecko.go) (HTTP only); [`cache.go`](internal/pricing/cache.go) (`NewCachingClient`, `NewCachingCryptoFetcher`, TTL from `RATES_CACHE_TTL_SECONDS` in [`Run`](internal/server/run.go)); [`mock.go`](internal/pricing/mock.go) implements `Client` / `CryptoPriceFetcher`.
- [x] **10. Extract blockchain RPC client usage** — All `callBlockchain`-style usage behind one `Blockchain` interface with timeouts and metrics. **Done:** [`internal/blockchain`](internal/blockchain/) (`Blockchain` interface, `RPCClient` alias, [`GetBlockRPCClient`](internal/blockchain/rpc_client.go)); [`callBlockchain`](internal/server/getusdperfiat.go) routes all RPC through `blockchainForCall`, default deadline [`defaultBlockchainRPCTimeout`](internal/server/getusdperfiat.go), [`metrics.RecordBlockchainRPC`](internal/metrics/metrics.go); legacy duplicate HTTP path removed.
- [x] **11. Email sending behind interface** — `EmailSender` interface; SMTP as one implementation; no-op for tests. **Done:** [`internal/email/email_sender.go`](internal/email/email_sender.go) (`EmailSender`); [`internal/email/smtp_sender.go`](internal/email/smtp_sender.go) (`SMTPSender`); [`internal/email/noop_sender.go`](internal/email/noop_sender.go) (`NoopEmailSender`); [`internal/email/email.go`](internal/email/email.go) (`Service` uses `EmailSender`); tests in [`internal/email/noop_sender_test.go`](internal/email/noop_sender_test.go).
- [x] **12. News service as standalone module** — Already partially there; ensure no `main` imports for business rules. **Done:** [`cmd/server/main.go`](cmd/server/main.go) imports only [`internal/server`](internal/server/); [`internal/news`](internal/news/) owns wiring via [`NewServiceFromConfig`](internal/news/wire.go) (used in [`Run`](internal/server/run.go)); [`internal/news/doc.go`](internal/news/doc.go) documents the boundary; tests in [`internal/news/wire_test.go`](internal/news/wire_test.go).
- [x] **13. PDF/CSV export in `internal/export`** — Isolate gofpdf and CSV building from HTTP handlers. **Done:** [`internal/export`](internal/export/) (`WritePortfolioPDF`, `WritePortfolioHoldingsCSV`, `WriteBlocksCSVHeader` / `WriteBlockRow`, `WriteTransactionsCSVHeader` / `WriteTransactionRow`, `Float64`/`String`, [`Version`](internal/export/meta.go) + limits); handlers in [`updatewatchlistentryhandler.go`](internal/server/updatewatchlistentryhandler.go) + [`portfolio_export.go`](internal/server/portfolio_export.go); JSON export meta uses [`export.Version`](internal/export/meta.go) in [`searchblockchain.go`](internal/server/searchblockchain.go); tests in [`internal/export/export_test.go`](internal/export/export_test.go).
- [x] **14. Consistent error types** — Domain errors map to stable API codes; avoid stringly-typed errors in hot paths. **Done:** [`internal/apperrors`](internal/apperrors/) (`ErrNotFound` sentinel, stable [`Code*`](internal/apperrors/apperrors.go) constants, typed [`Error`](internal/apperrors/apperrors.go) with `New`/`Wrap`); [`server.ErrNotFound`](internal/server/init.go) aliases [`apperrors.ErrNotFound`](internal/apperrors/apperrors.go); [`errorResponseFrom`](internal/server/getusdperfiat.go) maps domain errors for search/export; [`handleError`](internal/server/getusdperfiat.go) no longer returns raw `err.Error()` for 5xx; tests in [`internal/apperrors/apperrors_test.go`](internal/apperrors/apperrors_test.go).
- [x] **15. Version internal APIs** — Prefer `internal/...` packages; document what is stable for future extraction. **Done:** [docs/INTERNAL_APIS.md](docs/INTERNAL_APIS.md).

## Phase 12 — Testing & quality

- [x] **16. Set coverage targets per package** — e.g. `internal/news` ≥60%, handlers ≥40%; enforce in CI with gradual ratchet. **Done:** [`scripts/coverage_thresholds.txt`](scripts/coverage_thresholds.txt) + [`scripts/check_coverage.sh`](scripts/check_coverage.sh); CI step “Enforce per-package coverage (ratchet)”; README documents ratchet vs roadmap goals.
- [x] **17. Handler tests with `httptest` + mocks** — Every exported route group has at least smoke + one error path test. **Done:** [`internal/server/route_groups_v1_test.go`](internal/server/route_groups_v1_test.go) exercises `/api/v1` groups (explorer, feedback, news, user notifications/alerts/portfolio export) plus health/readiness; existing [`auth_test.go`](internal/server/auth_test.go), [`admin_api_test.go`](internal/server/admin_api_test.go), [`portfolio_watchlist_test.go`](internal/server/portfolio_watchlist_test.go), [`main_test.go`](internal/server/main_test.go) / [`service_test.go`](internal/server/service_test.go) cover auth, admin, portfolio/watchlist CRUD, legacy `/api/search`, and `SetExplorerService` mock.
- [x] **18. Table-driven tests for validation** — Password policy, pagination, portfolio/watchlist validation in compact tables. **Done:** [`internal/server/validation_table_test.go`](internal/server/validation_table_test.go) (`isStrongPassword`, `validateWatchlistEntry`); [`internal/apiutil/pagination_test.go`](internal/apiutil/pagination_test.go) (`ParsePagination`); expanded portfolio + new watchlist tables in [`portfolio_watchlist_test.go`](internal/server/portfolio_watchlist_test.go).
- [x] **19. Golden files for JSON responses** — Stable snapshots for critical list/detail payloads (optional but high value). **Done:** [`internal/server/golden_json_test.go`](internal/server/golden_json_test.go) + [`internal/server/testdata/golden/`](internal/server/testdata/golden/) (`search_success`, `search_error_missing_query`, `news_symbol`, `readyz`); normalize JSON and strip volatile keys (`correlation_id`, `timestamp`, `published_at`); refresh with `UPDATE_GOLDEN=1 go test ./internal/server -run TestGoldenJSON -count=1`.
- [x] **20. Redis integration tests** — Use miniredis or testcontainers for tests that need real Redis semantics beyond mocks. **Done:** [`internal/repos/redis_integration_test.go`](internal/repos/redis_integration_test.go) exercises portfolio round-trip, session TTL via `miniredis` `FastForward`, watchlist `Keys`/count, and session+CSRF delete; [`redisTestClient`](internal/repos/redis_integration_test.go) uses miniredis by default or dials real Redis when `BLOCKCHAIN_EXPLORER_TEST_REDIS=integration` (see [`internal/redistest`](internal/redistest/redistest.go)). No Docker/testcontainers dependency; optional real Redis matches production semantics.
- [x] **21. Contract tests for external APIs** — Record/replay or VCR-style for CoinGecko/news when feasible; strict timeouts. **Done:** [`internal/pricing/coingecko_contract_test.go`](internal/pricing/coingecko_contract_test.go) + [`internal/pricing/testdata/contracts/`](internal/pricing/testdata/contracts/) (replay via `httptest`; `CoinGeckoClient.BaseURL`); [`internal/news/thenewsapi_contract_test.go`](internal/news/thenewsapi_contract_test.go) + [`internal/news/testdata/contracts/`](internal/news/testdata/contracts/) (replay + 429 → `ErrRateLimited`); Resty `SetTimeout` for sub-second failure tests (no live HTTP).
- [x] **22. Load test script** — k6 or `vegeta` for `/api/search`, auth, and one heavy dashboard path; store baseline numbers. **Done:** [`scripts/loadtest/k6.js`](scripts/loadtest/k6.js) (parallel scenarios: `/api/search`, `POST /api/v1/login`, heavy `GET /api/v1/search/advanced`); [`scripts/loadtest/run-vegeta.sh`](scripts/loadtest/run-vegeta.sh) (GET `/api/search`, advanced search, `/dashboard`); baselines in [`scripts/loadtest/BASELINE.md`](scripts/loadtest/BASELINE.md); README links under **Load testing**.
- [x] **23. Fuzz tests** — `go test -fuzz` on parsers: txid, addresses, query params. **Done:** [`internal/server/fuzz_test.go`](internal/server/fuzz_test.go) (`FuzzIsValidTransactionID`, `FuzzIsValidAddress`, `FuzzIsValidBlockHeight`); [`internal/apiutil/fuzz_test.go`](internal/apiutil/fuzz_test.go) (`FuzzParsePagination`, `FuzzParseSort`). Query fuzz builds URLs via `url.ParseQuery` + `URL.RawQuery` so `httptest.NewRequest` never sees malformed request lines. Run one target at a time, e.g. `go test ./internal/server -fuzz=FuzzIsValidTransactionID -fuzztime=30s`.
- [x] **24. Race detector in CI** — `go test -race` on a schedule or on main; fix races found. **Done:** [`.github/workflows/race.yml`](.github/workflows/race.yml) runs `go test -race -count=1 ./...` on push to `main`/`master`, weekly (Mondays 06:00 UTC), and `workflow_dispatch`; README [Race detector (CI)](README.md#race-detector-ci). Local run: `go test -race -count=1 ./...` (passes; no races reported at time of addition).
- [x] **25. Benchmark hot paths** — `Benchmark` for search, cache key build, JSON marshal of large lists. **Done:** [`internal/repos/bench_test.go`](internal/repos/bench_test.go) (explorer `Prefix*` + portfolio/watchlist/session keys); [`internal/server/bench_test.go`](internal/server/bench_test.go) (`BenchmarkSearchBlockchain_*CacheHit`, `BenchmarkJSONMarshalSearchPayloadLarge`, `BenchmarkJSONMarshalAdvancedSearchEnvelopeLarge`). Run: `go test ./internal/repos ./internal/server -bench=. -benchmem -run=^$`.
- [x] **26. E2E tests (Playwright)** — Already have playwright dep; add CI job for critical user flows (login, search, portfolio). **Done:** [`@playwright/test`](package.json) + [`playwright.config.ts`](playwright.config.ts); [`e2e/critical.spec.ts`](e2e/critical.spec.ts) (login as dev admin + portfolios section, client-side search validation); [`.github/workflows/e2e.yml`](.github/workflows/e2e.yml) (Redis service, `go build`, server, `npx playwright install --with-deps chromium`); README [E2E tests (Playwright)](README.md#e2e-tests-playwright).
- [x] **27. Mutation testing (optional)** — `go-mutesting` or similar on pure packages to find weak tests. **Done:** [Gremlins](https://github.com/go-gremlins/gremlins) via [`scripts/mutation_test.sh`](scripts/mutation_test.sh) on `internal/apperrors`, `internal/correlation`, `internal/apiutil` (excludes `fuzz_test.go`); install `go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1`; optional CI [`.github/workflows/mutation.yml`](.github/workflows/mutation.yml) (`workflow_dispatch`); README [Mutation testing (optional)](README.md#mutation-testing-optional).
- [x] **28. Chaos tests (optional)** — toxiproxy or similar for Redis/network flakiness in staging. **Done:** [Toxiproxy](https://github.com/Shopify/toxiproxy) stack in [`scripts/chaos/docker-compose.yml`](scripts/chaos/docker-compose.yml) + [`scripts/chaos/toxiproxy-bootstrap.sh`](scripts/chaos/toxiproxy-bootstrap.sh); guide [`docs/CHAOS_TESTING.md`](docs/CHAOS_TESTING.md); optional **`REDIS_PORT`** ([`internal/config/config.go`](internal/config/config.go) `RedisPort`, [`internal/server/run.go`](internal/server/run.go) `RedisAddr`, [`internal/redistest`](internal/redistest/redistest.go)) so the app can use a proxied Redis on e.g. `localhost:6380` without conflicting with a local Redis on `6379`.

## Phase 13 — Security

- [x] **29. Threat model document** — STRIDE-lite: who attacks, what assets, what mitigations exist. **Done:** [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) (assets, threat actors, STRIDE-lite matrix with pointers to CSRF, rate limits, sessions, roles, metrics token, config validation); README [Documentation](README.md#documentation).
- [x] **30. Security headers audit** — Verify CSP, HSTS (behind TLS), X-Frame-Options, etc. against latest OWASP recommendations. **Done:** [`internal/server/security_headers.go`](internal/server/security_headers.go) + [`internal/server/security_headers_test.go`](internal/server/security_headers_test.go); HSTS via `HSTS_MAX_AGE_SECONDS` / `HSTS_INCLUDE_SUBDOMAINS` ([`internal/config/config.go`](internal/config/config.go)); checklist [docs/SECURITY_HEADERS.md](docs/SECURITY_HEADERS.md).
- [x] **31. CSRF token rotation policy** — Document and test session expiry + CSRF invalidation on password change. **Done:** [docs/CSRF_AND_SESSIONS.md](docs/CSRF_AND_SESSIONS.md); `PATCH /api/v1/user/password` + CSRF rotation ([`changePasswordHandler`](internal/server/updateprofilehandler.go)); Redis-authoritative session validation ([`validateSession`](internal/server/init.go)); session ids use unpadded base64; tests in [`internal/server/auth_test.go`](internal/server/auth_test.go).
- [x] **32. Rate limit bypass review** — Ensure metrics and health endpoints cannot be abused for DoS; document exemptions. **Done:** [docs/RATE_LIMITS.md](docs/RATE_LIMITS.md); `/healthz` + `/readyz` exempt from global limit; unauthenticated `GET /metrics` uses `METRICS_RATE_LIMIT_PER_IP` (default 120); [`rateLimitMiddleware`](internal/server/updateprofilehandler.go); tests [`internal/server/rate_limit_test.go`](internal/server/rate_limit_test.go).
- [x] **33. Input size limits everywhere** — Max body size, max JSON depth, max CSV rows for export. **Done:** [docs/INPUT_LIMITS.md](docs/INPUT_LIMITS.md); [`requestBodyLimitsMiddleware`](internal/server/request_limits.go) + [`apiutil.ValidateJSONDepth`](internal/apiutil/json_depth.go); config `MAX_REQUEST_BODY_BYTES`, `MAX_JSON_DEPTH`, `EXPORT_MAX_*_CSV_ROWS`; CSV caps [`effectiveBlockCSVRowCap`](internal/server/updatewatchlistentryhandler.go).
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
