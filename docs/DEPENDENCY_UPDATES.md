# Dependency update policy

This project uses **[GitHub Dependabot](https://docs.github.com/en/code-security/dependabot/dependabot-version-updates/about-dependabot-version-updates)** version updates (not the optional Dependabot security alerts-only mode—enable **Dependabot alerts** in the repository **Settings → Code security** if you want GitHub to surface known CVEs on dependencies).

## Configuration

| Item | Location |
|------|----------|
| Dependabot version updates | [`.github/dependabot.yml`](../.github/dependabot.yml) |
| Go modules | `go.mod` / `go.sum` |
| Frontend toolchain | `package.json` / `package-lock.json` (committed for reproducible `npm ci` and scans) |
| GitHub Actions pins | Workflow files under [`.github/workflows/`](../.github/workflows/) |

Updates are **scheduled monthly** and **grouped per ecosystem** (one grouped pull request for Go modules, one for npm, one for Actions) so review stays manageable.

## Monthly review

At least **once per month** (for example the first week after Dependabot opens its PRs):

1. **Triage** open Dependabot PRs and linked CI runs.
2. **Merge** when the full **CI** workflow is green, including:
   - **Lint and tests** (`go test`, coverage ratchet, frontend CSS build).
   - **Security** job: **gosec** and **Trivy** filesystem scan (`severity: CRITICAL,HIGH`, `ignore-unfixed: true`—see [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)).
   - **Docker** job: image build plus **Trivy** image scan with the same severity rules.
3. If a bump **fails Trivy** or tests: inspect the finding, check for a newer patch release, or **defer** and open an issue to track (do not merge a change that leaves known fixable HIGH/CRITICAL issues unaddressed without an explicit exception documented in the issue).

## Relationship to Trivy “gates”

CI is configured so **fixable** HIGH and CRITICAL vulnerabilities reported by Trivy fail the workflow (`exit-code: 1`). Keeping dependency updates current is the primary way those gates stay **green** over time: Dependabot proposes bumps; maintainers merge when scans and tests pass.

## Manual updates

Outside Dependabot, follow the same rule: run the checks in [README.md § Security scanning (CI)](../README.md#security-scanning-ci) locally or push a branch and confirm CI before merging.
