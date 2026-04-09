# Monitoring: dashboards and alerts

This folder describes how to observe **Bitcoin Explorer** in production using **Prometheus** (scrape `/metrics`) and **Grafana** (dashboards). The app exposes Prometheus metrics when `METRICS_ENABLED` is true (default); see root `README` or config for `METRICS_TOKEN` (Bearer or `X-Metrics-Token`) when protecting the endpoint.

## Quick wiring

1. **Scrape** the app’s `/metrics` from Prometheus (see `prometheus/prometheus-scrape-example.yml`).
2. **Import** the Grafana dashboard JSON (`grafana/dashboards/blockchain-explorer-overview.json`) or recreate panels using the PromQL below.
3. **Load** alert rules into Prometheus (`prometheus/prometheus-alerts.yml`) and route notifications through **Alertmanager** (`prometheus/alertmanager-escalation-example.yml`).

Application listens on **`:8080`** by default (`GET /metrics`). Adjust host/port for your deployment.

---

## Key metrics (reference)

| Metric | Labels | Use |
|--------|--------|-----|
| `http_requests_total` | `method`, `handler`, `status` | Traffic and **error rate** (status 4xx/5xx). |
| `http_request_duration_seconds` | `method`, `handler` | **Latency** (histogram; use `_bucket` / `histogram_quantile`). |
| `explorer_cache_events_total` | `layer`, `outcome` | **Cache hit ratio** by layer (e.g. `rates`, `news`, `search_etag`). |
| `explorer_background_job_runs_total` | `job` | Background loop activity (`prefetch`, `metrics_charts`, `price_alerts`). |
| `explorer_background_job_errors_total` | `job`, `class` | Prefetch failures (`blocks`, `transactions`). |
| `explorer_prefetch_last_success_unixtime` | — | **Staleness** of last good prefetch (compare to `time()`). |
| `explorer_alert_eval_duration_seconds` | — | Price-alert evaluation duration. |
| `explorer_alert_eval_triggered_total` | — | Cumulative in-app price alerts fired. |
| `explorer_blockchain_rpc_calls_total` | `method`, `status` | JSON-RPC to blockchain provider. |
| `explorer_blockchain_rpc_duration_seconds` | `method` | RPC latency. |
| `explorer_outbound_circuit_breaker_transitions_total` | `host`, `from_state`, `to_state` | Per-upstream breaker state changes ([`internal/outboundbreaker`](../internal/outboundbreaker/transport.go)). |
| `explorer_outbound_circuit_breaker_rejections_total` | `host`, `reason` | Requests short-circuited (`open` / `half_open`). |
| `explorer_email_queue_depth` | — | Buffered outbound emails waiting in-process ([`internal/email`](../internal/email/email.go)). |
| `explorer_email_enqueue_dropped_total` | `reason` | Emails not queued (e.g. `queue_full`); see admin `email_queue` on `/api/v1/admin/status`. |
| `explorer_email_dead_letter_entries` | — | Rows retained in the in-process dead-letter ring. |

**External APIs (GetBlock, CoinGecko, news):** the app does not yet export separate histograms per upstream. Treat **HTTP 5xx rate** and **latency** on routes that call those dependencies as a proxy (e.g. `/api/v1/search`, `/api/v1/rates`, `/api/v1/network-status`). For deeper visibility, add custom metrics or use Sentry performance traces.

---

## Grafana — suggested panels

Use a Prometheus data source. Replace `job="$job"` with your scrape job name (e.g. `blockchain-explorer`).

### Request rate

```promql
sum(rate(http_requests_total[5m])) by (status)
```

### Error rate (5xx / all requests)

```promql
sum(rate(http_requests_total{status=~"5.."}[5m]))
/
clamp_min(sum(rate(http_requests_total[5m])), 1e-9)
```

### Latency — p95 (all routes)

```promql
histogram_quantile(0.95,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, handler)
)
```

### Latency — p99 (all routes)

```promql
histogram_quantile(0.99,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, handler)
)
```

### Cache hit ratio — rates endpoint (example)

```promql
sum(rate(explorer_cache_events_total{layer="rates", outcome="hit"}[15m]))
/
clamp_min(
  sum(rate(explorer_cache_events_total{layer="rates"}[15m])),
  1e-9
)
```

### Prefetch health — seconds since last success

```promql
time() - explorer_prefetch_last_success_unixtime
```

### Background prefetch errors (per second)

```promql
sum(rate(explorer_background_job_errors_total{job="prefetch"}[5m])) by (class)
```

---

## Alert thresholds (starting points)

Tune for your traffic profile. Values assume steady production load.

| Alert | Condition (example) | Severity | Notes |
|-------|---------------------|----------|--------|
| **HighHTTP5xxRate** | 5xx share of requests > **5%** for **10m** | warning | May indicate app or upstream RPC issues. |
| **CriticalHTTP5xxRate** | 5xx share > **20%** for **5m** | critical | Likely user-visible outage. |
| **HighLatencyP99** | p99 latency > **3s** for **15m** | warning | Investigate slow handlers or providers. |
| **PrefetchStale** | `time() - explorer_prefetch_last_success_unixtime` > **120s** | warning | Prefetch failing or stalled. |
| **PrefetchErrors** | `rate(explorer_background_job_errors_total{job="prefetch"}[10m])` > **0.05** /s | warning | Repeated block/tx prefetch failures. |
| **AlertEvalSlow** | p95 `explorer_alert_eval_duration_seconds` > **30s** for **15m** | warning | Redis scan or pricing slowness. |

Rules are codified in `prometheus/prometheus-alerts.yml` with recording helpers where useful.

---

## Escalation paths

Define ownership in your org; below is a template.

| Severity | Response time | Who | Channels |
|----------|----------------|-----|----------|
| **P1 — Critical** (SLO breach, full outage) | **15 min** | On-call engineer | PagerDuty / phone → Slack `#incidents` → status page |
| **P2 — High** (degraded, elevated 5xx or latency) | **1 h** | Primary on-call | Slack `#blockchain-explorer-alerts` + ticket |
| **P3 — Warning** (threshold drift, prefetch stale) | **Next business day** | Team backlog | Email digest or Slack |

**Handoff:** Document a **runbook** link in each alert annotation (playbook URL). After mitigation, schedule a **post-incident review** if P1/P2.

Example Alertmanager flow:

1. **route** `severity=critical` → PagerDuty + Slack.
2. **route** `severity=warning` → Slack only (business hours).
3. **inhibit** warning if critical for the same `alertname` (optional).

See `prometheus/alertmanager-escalation-example.yml` for a commented skeleton.

---

## Validation

If you install Prometheus `promtool`:

```bash
promtool check rules prometheus/prometheus-alerts.yml
```

---

## Files in this directory

| Path | Purpose |
|------|---------|
| `prometheus/prometheus-scrape-example.yml` | Example static scrape config snippet. |
| `prometheus/prometheus-alerts.yml` | Recording + alerting rules. |
| `prometheus/alertmanager-escalation-example.yml` | Alertmanager route/receiver skeleton. |
| `grafana/dashboards/blockchain-explorer-overview.json` | Importable Grafana dashboard; on import, pick your Prometheus datasource. |
| `scripts/check-alert-rules.sh` | Runs `promtool check rules` when `promtool` is available. |

---

## SLO sketch (optional)

- **Availability:** `1 - (5xx requests / all requests)` over 30d rolling target **99.9%**.
- **Latency:** p99 &lt; **2s** on core read APIs under normal load.

Track error budget burn in Grafana (ratio of alerts firing vs. window).
