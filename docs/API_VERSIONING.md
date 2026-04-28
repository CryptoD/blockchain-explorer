# API versioning and deprecation policy

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **65**. It defines how the project versions its HTTP JSON API, how deprecations are announced, and which **response headers** operators and clients should rely on when phasing out routes.

**Implementation mapping:** route registration in [`internal/server/routes.go`](../internal/server/routes.go); machine-readable contract in [`openapi.yaml`](../openapi.yaml).

---

## Versioning model

| Surface | Path prefix | Status |
|---------|-------------|--------|
| **Preferred** | `/api/v1/` | Current contract; new integrations and SDKs should target only this tree. |
| **Legacy** | `/api/` (same handlers as today) | **Deprecated** for new work; retained for backward compatibility. Migrate to `/api/v1/` equivalents. |

Breaking changes (removing fields, renaming parameters, changing semantics without a new path) ship only by introducing a **new path prefix** (for example a future `/api/v2/`) or by **documented sunset** of the old behavior, never silently.

---

## Minimum notice before removal or breaking change

Unless a **critical security** or **compliance** exception is documented in the release notes:

1. **Publication:** At least **90 calendar days** must pass between **public announcement** and the **effective removal** or incompatible change.
2. **Announcement** must be visible to API consumers: GitHub **release notes** (or project advisory), and a **`CHANGELOG.md`** entry once that file exists ([ROADMAP](../ROADMAP_TO_100.md) task **72**).
3. **`openapi.yaml`:** Deprecated operations are marked with OpenAPI **`deprecated: true`** and a description that names the **replacement** route (if any).

Extensions or optional fields may be added without a new API version when they are **backward compatible** (additive only).

---

## Sunset and deprecation headers (HTTP)

When a specific route or legacy prefix is in a deprecation window, responses **SHOULD** include the following headers so automated clients can detect timelines without parsing HTML.

| Header | Specification | Purpose |
|--------|-----------------|---------|
| **`Deprecation`** | [RFC 9745](https://www.rfc-editor.org/rfc/rfc9745.html) | Signals that the resource is deprecated. May be a boolean (`true`) or an **HTTP-date** indicating when deprecation started. |
| **`Sunset`** | [RFC 8594](https://www.rfc-editor.org/rfc/rfc8594.html) | **HTTP-date (GMT)** after which the resource **may** be removed or no longer honor the previous contract. Must be **on or after** the end of the minimum notice period from the public announcement. |
| **`Link`** | [RFC 8288](https://www.rfc-editor.org/rfc/rfc8288.html) | Pointer to the **successor** resource, e.g. `Link: </api/v1/search>; rel="successor-version"` (or `rel="alternate"` with a clear description in docs). |

**Client guidance:** Log or monitor `Deprecation` and `Sunset`; treat the sunset date as the deadline to switch to the replacement URL. If `Sunset` is absent, follow published dates in **release notes** and `openapi.yaml` only.

**Server guidance:** When implementing deprecation in code, set these headers consistently for all success and typical error responses on the deprecated route, until removal.

---

## Relationship to roadmap item “API deprecation communications”

Template copy for downstream emails or partner notices is covered by [ROADMAP](../ROADMAP_TO_100.md) task **94** (**API deprecation communications**); this policy is the contractual baseline (notice duration + headers) that communications should cite.
