# Internal packages and API stability

Go’s `internal/` rule means code under `github.com/CryptoD/blockchain-explorer/internal/...` is **not importable** from outside this module. Within the repo we still distinguish **contract-first packages** (good candidates to lift into a shared library or `pkg/` later) from **application shell** code.

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **15**. Update it when you add a new `internal/` package or change a stability tier.

**Inventory note:** The table below includes packages added through roadmap work through task **61** (e.g. [`internal/idempotency`](../internal/idempotency/), [`internal/featureflags`](../internal/featureflags/), [`internal/outboundbreaker`](../internal/outboundbreaker/), [`internal/retrybudget`](../internal/retrybudget/), email queue + dead-letter behavior in [`internal/email`](../internal/email/email.go)). Extend the table when new `internal/` boundaries appear.

---

## Stability tiers

| Tier | Meaning |
|------|--------|
| **Stable** | Exported types and functions are intentional contracts; breaking changes should be rare and called out in PRs / changelog. |
| **Evolving** | Useful boundaries exist but names, metrics labels, or helper signatures may change as observability and HTTP helpers evolve. |
| **Shell** | Tied to HTTP wiring, globals, and process lifecycle—not a library API. |

---

## Package inventory

| Package | Tier | Stable surface (for future extraction) |
|---------|------|----------------------------------------|
| [`internal/blockchain`](../internal/blockchain/) | Stable | [`Blockchain`](../internal/blockchain/rpc_client.go) (alias `RPCClient`), [`GetBlockRPCClient`](../internal/blockchain/rpc_client.go), [`MockRPCClient`](../internal/blockchain/mock.go) |
| [`internal/pricing`](../internal/pricing/) | Stable | [`Client`](../internal/pricing/interfaces.go), [`AssetPricer`](../internal/pricing/interfaces.go), [`CompositePricer`](../internal/pricing/assets.go), [`NewCachingClient`](../internal/pricing/cache.go) / [`NewCachingCryptoFetcher`](../internal/pricing/cache.go), fiat helpers in [`fiat.go`](../internal/pricing/fiat.go) / [`fiat_convert.go`](../internal/pricing/fiat_convert.go) |
| [`internal/email`](../internal/email/) | Stable | [`EmailSender`](../internal/email/email_sender.go), [`SMTPSender`](../internal/email/smtp_sender.go), [`NoopEmailSender`](../internal/email/noop_sender.go), [`Service`](../internal/email/email.go) (queue + in-process dead letter when buffer full; [`QueueAdminSnapshot`](../internal/email/email.go) for admin) |
| [`internal/news`](../internal/news/) | Stable | [`Service`](../internal/news/service.go), [`Provider`](../internal/news/provider.go), [`NewServiceFromConfig`](../internal/news/wire.go), [`Cache`](../internal/news/cache.go) |
| [`internal/export`](../internal/export/) | Stable | [`Version`](../internal/export/meta.go), export limits, [`WritePortfolioPDF`](../internal/export/portfolio_pdf.go), portfolio/blocks/tx CSV helpers |
| [`internal/apperrors`](../internal/apperrors/) | Stable | [`ErrNotFound`](../internal/apperrors/apperrors.go), [`Error`](../internal/apperrors/apperrors.go), [`Code*`](../internal/apperrors/apperrors.go) constants |
| [`internal/repos`](../internal/repos/) | Stable | [`Stores`](../internal/repos/stores.go), typed repos, [`keys`](../internal/repos/keys.go) |
| [`internal/config`](../internal/config/) | Stable | [`Config`](../internal/config/config.go), [`Load`](../internal/config/config.go), [`Validate`](../internal/config/config.go) |
| [`internal/outboundbreaker`](../internal/outboundbreaker/) | Evolving | [`WrapRoundTripper`](../internal/outboundbreaker/transport.go) per-host [`gobreaker`](https://github.com/sony/gobreaker) on shared outbound HTTP |
| [`internal/retrybudget`](../internal/retrybudget/) | Evolving | [`WithAttemptBudget`](../internal/retrybudget/budget.go), [`WrapRoundTripper`](../internal/retrybudget/transport.go) for optional per-inbound caps |
| [`internal/idempotency`](../internal/idempotency/) | Evolving | Redis-backed export idempotency ([`Store`](../internal/idempotency/store.go), [`RequestFingerprint`](../internal/idempotency/fingerprint.go)) |
| [`internal/featureflags`](../internal/featureflags/) | Evolving | News and price-alert enablement ([`Resolver`](../internal/featureflags/featureflags.go)); Redis keys `feature:news`, `feature:price_alerts` ([`docs/FEATURE_FLAGS.md`](FEATURE_FLAGS.md)) |
| [`internal/metrics`](../internal/metrics/) | Evolving | Histogram/counter names and [`Middleware`](../internal/metrics/metrics.go) labels—treat as operational contract, not semver |
| [`internal/correlation`](../internal/correlation/) | Evolving | Request ID helpers and header names |
| [`internal/sentryutil`](../internal/sentryutil/) | Evolving | `Init` and options (see package) |
| [`internal/apiutil`](../internal/apiutil/) | Evolving | Pagination/sort helpers tied to query params |
| [`internal/logging`](../internal/logging/) | Evolving | Component names and field keys |
| [`internal/redisstore`](../internal/redisstore/) | Evolving | Thin alias over `redis.Cmdable` |
| [`internal/redistest`](../internal/redistest/) | Shell | Test-only Redis integration helpers |
| [`internal/server`](../internal/server/) | Shell | [`Run`](../internal/server/run.go), handlers, [`Dependencies`](../internal/server/deps.go); test hooks [`Set*Service`](../internal/server/service_interfaces.go), [`ResetDefaultServices`](../internal/server/service_defaults.go) |

---

## Rules for contributors

1. **Prefer** importing domain logic from the **Stable** packages above instead of duplicating behavior in `internal/server`.
2. **Do not** introduce `import "…/internal/server"` from other `internal/*` packages (keeps the dependency DAG acyclic; see [BOUNDED_CONTEXTS.md](BOUNDED_CONTEXTS.md)).
3. **Stable** packages: treat exported identifiers as API; prefer additive changes; document intentional breaks.
4. **HTTP JSON** export metadata uses [`export.Version`](../internal/export/meta.go) where applicable; that string is the archival format hint, not Go module semver.

---

## Relation to bounded contexts

Domain boundaries and allowed dependency directions are described in [BOUNDED_CONTEXTS.md](BOUNDED_CONTEXTS.md). This document focuses on **which packages are safe to treat as reusable contracts** inside the module and beyond if you later split libraries.
