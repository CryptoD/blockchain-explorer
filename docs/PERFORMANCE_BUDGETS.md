# Performance budgets

Targets for public HTML routes (home, explorer, symbols). Aligned with [Core Web Vitals](https://web.dev/vitals/) “good” thresholds and enforced in CI via [Lighthouse CI](https://github.com/GoogleChrome/lighthouse-ci) (optional job).

## Core Web Vitals targets

| Metric | Budget (good) | Lighthouse audit id | CI enforcement |
|--------|---------------|---------------------|----------------|
| **LCP** (Largest Contentful Paint) | ≤ **2.5 s** | `largest-contentful-paint` | warn |
| **CLS** (Cumulative Layout Shift) | ≤ **0.1** | `cumulative-layout-shift` | error |
| TBT (Total Blocking Time) | ≤ 300 ms | `total-blocking-time` | warn |
| Speed Index | ≤ 3.4 s | `speed-index` | warn |
| Performance score | ≥ 0.75 | `categories:performance` | warn |
| Accessibility score | ≥ 0.90 | `categories:accessibility` | warn |

CLS is enforced as **error** because layout stability should not regress on static shells. LCP and performance score use **warn** in CI so noisy runners do not block merges; tighten to `error` when baselines are stable.

## Pages under test

Configured in [`lighthouserc.cjs`](../lighthouserc.cjs):

- `/` — home / search entry
- `/bitcoin` — explorer shell
- `/symbols` — symbol search

## Local run

Prerequisites: Redis on `localhost:6379`, Go 1.24+, Node 18+.

```bash
npm ci
npm run build
go build -o explorer ./cmd/server
REDIS_HOST=localhost npm run test:lighthouse
```

Reports are written to `.lighthouseci/` (gitignored).

## CI

Workflow [`.github/workflows/lighthouse.yml`](../.github/workflows/lighthouse.yml) runs on pull requests and `workflow_dispatch`. It is **informational** (`continue-on-error: true`) so performance tracking does not block delivery; download the **lighthouse-report** artifact to inspect JSON/HTML output.

To make budgets blocking, remove `continue-on-error` from the workflow and/or change assertion levels in `lighthouserc.cjs` from `warn` to `error`.

## Related frontend practices

- Lazy-load Chart.js via `/dist/js/chart-*.js` (dashboard / metrics).
- Minified CSS via `npm run build:css`.
- Versioned/stamped HTML in `build/stamped/` for production Docker builds.
