# Retry budgets (outbound HTTP)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **56**. It describes how the app **limits retries** and **caps total outbound HTTP work** per inbound request so failures do not multiply into retry storms (especially with [circuit breakers](CIRCUIT_BREAKERS.md)).

---

## 1. Resty retry count (per logical outbound call)

The shared [`resty`](https://github.com/go-resty/resty) client ([`newRestyClientForConfig`](../internal/server/outbound_wire.go)) sets **`SetRetryCount(n)`** from configuration. Resty’s backoff runs at most **`n + 1` HTTP attempts** for a single `Get`/`Post`/… invocation (see [`request.go` in resty](https://github.com/go-resty/resty/blob/master/request.go)).

| Env | Default | Meaning |
|-----|---------|---------|
| `OUTBOUND_HTTP_RETRY_COUNT` | `3` | Max **retries** after the first attempt (`0` = no Resty retries). Valid **0–10**. |

This is the **primary** knob for “how many times we hammer the same upstream for one client call.” Lower it in unstable environments or when combined with aggressive caching.

---

## 2. Per-inbound-request attempt budget (optional)

When **`OUTBOUND_HTTP_INBOUND_ATTEMPT_BUDGET`** is **greater than zero**, middleware ([`inboundRetryBudgetMiddleware`](../internal/server/retry_budget_middleware.go)) attaches a **shared counter** to **`c.Request.Context()`** implemented by [`internal/retrybudget`](../internal/retrybudget/). The outbound [`http.RoundTripper`](../internal/retrybudget/transport.go) sits **under** Resty and **above** the TCP transport and [circuit breaker](../internal/outboundbreaker/transport.go): **each** successful scheduling of an outbound `RoundTrip` (including each Resty retry attempt) **consumes one unit** from the budget.

| Env | Default | Meaning |
|-----|---------|---------|
| `OUTBOUND_HTTP_INBOUND_ATTEMPT_BUDGET` | `0` | **Off.** No per-request cap. |
| | `> 0` | Max outbound **RoundTrip** calls for **this** incoming HTTP request, **if** handlers pass the Gin request context into Resty (`SetContext(ctx)`). |

**Important:**

- Code paths that use **`context.Background()`** (e.g. some background jobs) **do not** inherit the budget.
- A single JSON-RPC call can use up to **`OUTBOUND_HTTP_RETRY_COUNT + 1`** attempts; heavy handlers that chain many RPCs need a budget large enough for worst case, or they may see [`retrybudget.ErrBudgetExhausted`](../internal/retrybudget/budget.go).

---

## 3. Interaction with circuit breakers

Circuit breaking is **per upstream host**; retry budgets are **per inbound request** (when enabled) and **per Resty call** (retry count). Together they reduce **retry storms**: breakers stop traffic to a bad host; Resty retry limits and inbound budgets cap **how many tries** a single request can trigger.

---

## Related

- [`docs/CIRCUIT_BREAKERS.md`](CIRCUIT_BREAKERS.md)  
- [`internal/server/outbound_wire.go`](../internal/server/outbound_wire.go) (transport order: retry budget → circuit breaker → pooled transport)
