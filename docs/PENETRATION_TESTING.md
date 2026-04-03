# External penetration testing

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **40**. It describes **when** to commission third-party penetration tests and **how** to track remediation—not a substitute for a formal test plan or contract with an assessor.

## Cadence

| Trigger | Rationale |
|---------|-----------|
| **At least annually** | Catch regressions and new attack surface as the app and dependencies evolve. |
| **Before a major launch** | Major releases (new public surfaces, auth changes, large traffic expectations) warrant validation before go-live. |

“Major launch” is a product decision; examples include first production deployment at scale, a large marketing push, or shipping materially new features (e.g. new payment flows, OAuth, public APIs).

## Scope and environment

- Engage a **qualified external** firm or individual (independent of day-to-day development).
- Run tests against an environment that **matches production** configuration (TLS, Redis, rate limits, `APP_ENV`, secrets layout) without exposing real user data—typically **staging** or a dedicated **test** deployment with synthetic data.
- Define **rules of engagement** in writing: allowed IP ranges, time windows, out-of-scope systems, and emergency contacts.
- Automated checks in CI (**gosec**, **Trivy**, **Gitleaks**, etc.) are **complementary**; they do not replace manual/expert testing.

## Tracking findings

1. **File GitHub issues** for each finding (or grouped finding cluster) from the report. Use a label such as **`security`** and optionally **`pentest`** / **`pentest-YYYY`** for the engagement.
2. In the issue body, include: **title or ID from the report**, **severity**, **affected component or route**, and **recommended fix** (from the report or triage).
3. Link related PRs; close the issue when remediated and **re-tested** (or explicitly accepted as risk with documented sign-off).
4. Keep the **original report** in a secure location (not necessarily the public repo); the issue tracker holds the actionable work.

## Related documents

- [Threat model (STRIDE-lite)](THREAT_MODEL.md) — what we already assume and mitigate.
- [Security policy](../SECURITY.md) — vulnerability reporting for independent researchers.
- [Security HTTP headers](SECURITY_HEADERS.md) — CSP and other headers assessors will check.
