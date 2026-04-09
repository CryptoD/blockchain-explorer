# SQL injection (N/A) and Redis key safety

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) tasks **34** (key safety audit) and **48** (future Redis read scaling / cluster story).

---

## SQL injection — not applicable today

**Finding:** The codebase does **not** use a relational database or SQL for application state.

- No imports of `database/sql`, `gorm`, `sqlx`, `pgx`, SQLite drivers, or similar in production paths.
- User accounts, sessions, CSRF tokens, portfolios, watchlists, caches, and feedback use **Redis** ([`internal/repos`](../internal/repos/), [`internal/server`](../internal/server/)).

**If SQL is introduced later:**

- Use **parameterized queries** / prepared statements for all dynamic values.
- **Never** build queries with string concatenation of untrusted input (e.g. `"... WHERE id = '" + id + "'"`).
- Prefer ORM/query builders that bind parameters explicitly.

A **Postgres-oriented schema sketch** for moving durable user state off Redis (not implemented) lives in [POSTGRES_MIGRATION_SKETCH.md](POSTGRES_MIGRATION_SKETCH.md) ([ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **50**).

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
- **Backups and disaster recovery:** see [REDIS_BACKUP_AND_RESTORE.md](REDIS_BACKUP_AND_RESTORE.md) (RDB/AOF, restore outline, what user data is lost if Redis is wiped without backup).

---

## Future: Redis read scaling and cluster (task 48)

**Current architecture:** The server uses a **single** Redis endpoint (`REDIS_HOST`, `REDIS_PORT` in [`internal/config/config.go`](../internal/config/config.go)) and one shared client for **all** reads and writes. Connection pooling and timeouts are configurable there. There is **no** separate reader endpoint or cluster router in code today.

If Redis becomes the **bottleneck** (CPU, memory, connections, or command latency), use the following as a **decision ladder**—not a commitment to implement everything.

### 1. Scale up and tune (first step)

- Move to a **larger** instance or shard **non-Redis** work (RPC, pricing HTTP) before changing topology.
- Tune **`REDIS_POOL_SIZE`**, **`REDIS_MAX_ACTIVE_CONNS`**, and timeout env vars so the app does not open unbounded connections or block on a saturated pool.

### 2. High availability (failover), not read throughput

- **Redis Sentinel**, or a **managed HA** primary (e.g. AWS ElastiCache Multi-AZ, GCP Memorystore with HA, Azure Cache for Redis with zone redundancy) improves **uptime** when the primary fails.
- Clients still target **one writable primary** for mutations; this does **not** by itself multiply read capacity.

### 3. Read replicas (horizontal read scaling)

- **Replication** adds one or more **asynchronous replicas**. Managed products often expose a **reader endpoint** (e.g. ElastiCache **reader** DNS) distinct from the primary.
- **Risk for this application:** Many paths are **read-after-write** (sessions, CSRF tokens, portfolios, watchlists immediately after update). Replicas can **lag** the primary; routing those reads to a replica can cause **spurious 401s**, stale CSRF validation, or briefly stale user-visible state.
- **Reasonable future approach (requires code + config):**
  - Keep **all writes and all consistency-sensitive reads** on the **primary**.
  - Optionally introduce a **second** `redis.Client` (or a thin router) pointed at **replica(s)** only for operations where **eventual consistency is acceptable** (e.g. some **explorer cache** reads, **read-only admin introspection**, or other paths explicitly reviewed and tested under lag).
  - **Avoid** replica routing for: session lookup, rate-limit **`INCR`** / check-and-act patterns, and any handler that must see a write that just occurred in the same request or redirect flow.
- **Client libraries:** For a simple primary + replica pair, two endpoints and explicit **read vs write** routing in application code is the usual pattern. **Redis Cluster** clients use slot-aware routing; classic OSS replication without Cluster is often **two clients** or a **TCP proxy** in front of replicas, not a single magic URL.

### 4. Redis Cluster (horizontal data partitioning)

- **Redis Cluster** splits keyspace into **hash slots** (16 384 slots). Use when **data size** or **single-node command throughput** no longer fits one host.
- **Compatibility caveat for this repo:** The application uses **multi-key** commands in places (e.g. **`MGET`** batches for price-alert evaluation, pipelines, and any multi-key deletes). In Cluster mode, each multi-key command must address keys that hash to the **same slot**, or Redis returns **`CROSSSLOT`** errors.
- **Migration would require:** an audit of **all** multi-key operations and, where needed, **hash tags** in key names (e.g. `{user123}portfolio:...` and `{user123}watchlist:...` only if those keys must participate in the same multi-key op—design carefully to avoid accidental key colocation).
- Use a **cluster-enabled** go-redis configuration when moving to Cluster; re-test **SCAN**, **pipelines**, and background jobs under cluster semantics.

### Summary

| Approach | Solves | Does not solve |
|----------|--------|----------------|
| Bigger instance + pool tuning | Hot single node, connection storms | Dataset larger than one machine |
| HA / Sentinel / managed failover | Primary failure | Read throughput |
| Read replicas | Read-heavy, lag-tolerant paths | Strong read-your-writes everywhere |
| Redis Cluster | Sharded data + aggregate throughput | Without key design: safe multi-key ops |

Treat **replica routing** and **Cluster** as **explicit architecture work**: document which commands use which endpoint, acceptable replication lag, and add monitoring (replica lag, `CROSSSLOT` errors, pool usage).

---

## Related documents

- [THREAT_MODEL.md](THREAT_MODEL.md) — assets and Redis trust boundary.
- [INPUT_LIMITS.md](INPUT_LIMITS.md) — request body and JSON depth (task 33).
