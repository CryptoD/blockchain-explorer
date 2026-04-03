# SQL injection (N/A) and Redis key safety

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **34**. It records an **audit** of how the app persists data and how Redis keys are built.

---

## SQL injection — not applicable today

**Finding:** The codebase does **not** use a relational database or SQL for application state.

- No imports of `database/sql`, `gorm`, `sqlx`, `pgx`, SQLite drivers, or similar in production paths.
- User accounts, sessions, CSRF tokens, portfolios, watchlists, caches, and feedback use **Redis** ([`internal/repos`](../internal/repos/), [`internal/server`](../internal/server/)).

**If SQL is introduced later:**

- Use **parameterized queries** / prepared statements for all dynamic values.
- **Never** build queries with string concatenation of untrusted input (e.g. `"... WHERE id = '" + id + "'"`).
- Prefer ORM/query builders that bind parameters explicitly.

Re-audit when the first SQL-backed store is added.

---

## Redis: not “SQL injection,” but key and pattern discipline

Redis commands are issued through the **go-redis** client (`Get`, `Set`, `HSet`, `Scan`, …) as **structured API calls**, not by concatenating user input into a single command string. There is **no** Redis “query language” in the same sense as SQL.

Risks to manage are different:

| Risk | Mitigation in this repo |
|------|-------------------------|
| **Cross-tenant access** | Domain keys include **authenticated `username`** from the session (not raw client input) for user-owned data, or **validated** identifiers (see below). |
| **Key layout** | Central prefixes and builders in [`internal/repos/keys.go`](../internal/repos/keys.go) (`UserKey`, `PortfolioKey`, `SessionKey`, …). |
| **Explorer cache keys** | `address:` + address, `tx:` + txid, `block:` + height/hash in [`getAddressDetails` / `getTransactionDetails` / `getBlockDetails`](../internal/server/getusdperfiat.go) — only after **validation** (`isValidAddress`, `isValidTransactionID`, block height/hash checks in [`searchBlockchain`](../internal/server/searchblockchain.go)). |
| **`SCAN` `MATCH` patterns** | Patterns such as `alert:{username}:*` and `notification:{username}:*` ([`listPriceAlertsHandler`](../internal/server/updateprofilehandler.go), notifications) embed **username** from the session. **Registration/login** restrict usernames to `^[a-zA-Z0-9_.-]+$` ([`registerHandler`](../internal/server/updatepricealerthandler.go)), which **excludes** Redis glob metacharacters (`*`, `?`, `[`) that could broaden a `MATCH` pattern unexpectedly. |
| **Lua scripts (`EVAL`)** | **Not used** in this repository. If added, pass keys and arguments via the client’s `Keys` / `Args` (or equivalent), not by interpolating user input into script source. |

---

## Operational assumptions

- Redis is reachable only from the **application network** (see [THREAT_MODEL.md](THREAT_MODEL.md)).
- Compromise of Redis still exposes data at rest; this document does not replace **encryption at rest** or **Redis ACLs** where required by policy.

---

## Related documents

- [THREAT_MODEL.md](THREAT_MODEL.md) — assets and Redis trust boundary.
- [INPUT_LIMITS.md](INPUT_LIMITS.md) — request body and JSON depth (task 33).
