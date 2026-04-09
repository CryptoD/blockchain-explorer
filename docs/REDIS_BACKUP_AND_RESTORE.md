# Redis backup and restore runbook

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **59**. It describes **Redis persistence options** (RDB vs AOF), what to expect when **restoring** from snapshots, and **what is lost** if Redis data is **destroyed with no backup**.

**Scope:** The production server stores **application state in Redis** (users, sessions, portfolios, caches, etc.). There is **no relational database** in the application code path today ([`docs/SQL_AND_REDIS_SAFETY.md`](SQL_AND_REDIS_SAFETY.md)). Optional Postgres in [`docker-compose.yml`](../docker-compose.yml) is **not** used by the Go server for this app’s domain data; treat **Redis** as the system of record for operational recovery planning unless you have introduced another store.

---

## 1. Persistence strategies (RDB vs AOF)

Redis offers two durable representations on disk (plus combinations):

| Mechanism | What it is | Durability | Tradeoffs |
|-----------|------------|------------|-----------|
| **RDB** | Point-in-time **snapshot** of the dataset (`dump.rdb` or managed equivalent). | You can lose writes **since the last successful snapshot**. | Low runtime overhead; compact; good for **periodic** backups and fast restarts. Default `redis.conf` often includes `save 900 1`, `save 300 10`, etc. |
| **AOF** | **Append-only** log of mutating commands, replayed on startup. | With `appendfsync everysec`, at most **~1 second** of writes may be lost on a hard crash; `always` is stricter but slower. | Larger disk use; rewrite (`BGREWRITEAOF`) needed over time. |
| **RDB + AOF** (Redis 7+) | Both enabled; recovery uses AOF if present, else RDB. | Tunes between **snapshot cadence** and **log durability**. | Operational complexity; preferred for many production deployments when self-managing OSS Redis. |

**Managed Redis** (AWS ElastiCache, GCP Memorystore, Azure Cache for Redis, Redis Enterprise Cloud, etc.) exposes **automated backups**, **multi-AZ failover**, and retention policies—**prefer vendor docs** for RDB scheduling, AOF defaults, and restore procedures.

**Self-hosted (Docker / VM):**

- Ensure the data directory is on **durable storage** (volume, EBS, etc.), not ephemeral container storage.
- For `docker-compose`, the sample [`redis-data` volume](../docker-compose.yml) mounts Redis `/data`; the **official Redis image** still uses default **`save`** rules for RDB unless you supply a custom `redis.conf`. **AOF is not enabled by default**—evaluate `appendonly yes` for stricter durability.
- After changing persistence settings, **restart Redis** in a maintenance window and verify `INFO persistence` (or cloud backup status).

---

## 2. Backup procedures (outline)

Exact commands depend on your orchestration; patterns:

1. **Block writes or accept brief inconsistency** — For strict consistency, **stop application pods** (or put maintenance mode) so no new writes occur during snapshot export.
2. **Trigger or copy RDB** — `BGSAVE` / wait for periodic save / use managed “Create backup”.
3. **If using AOF** — Copy AOF after `BGREWRITEAOF` (optional) or rely on managed backup that includes AOF.
4. **Store artifacts** off-instance: object storage with **encryption**, **versioning**, and **IAM** scoped to restore roles.
5. **Document** backup time, Redis version, and app version for each artifact.

**`SAVE` vs `BGSAVE`:** Prefer **`BGSAVE`** (background fork) on live instances to avoid blocking the main thread; verify completion in logs.

---

## 3. Restore procedures (outline)

1. **Provision** a new Redis instance (same major version if possible) or **empty** the target data directory if restoring in place (destructive).
2. **Restore** vendor snapshot or place `dump.rdb` / AOF files per Redis documentation.
3. **Start Redis** and verify: `PING`, `INFO keyspace`, spot-check keys (see §4).
4. **Deploy application** with the same **`REDIS_HOST` / `REDIS_PORT`** (or update DNS to the restored endpoint).
5. **Validate** login, profile, one portfolio/watchlist read/write, and readiness (`GET /ready`).

**Downgrade / version jump:** Restoring an RDB from a **newer** Redis into an **older** server may fail; align versions or follow Redis release notes.

---

## 4. What lives in Redis (key families)

Authoritative prefixes and builders: [`internal/repos/keys.go`](../internal/repos/keys.go). Additional patterns appear in handlers (explorer cache, rates, news, rate limits).

| Category | Examples | If Redis is wiped **without** backup |
|----------|----------|--------------------------------------|
| **User accounts** | `user:{username}` | **Lost:** password hashes, profile (email, notification prefs). Users must **re-register** or operators **restore** from backup. |
| **Sessions & CSRF** | `session:{id}`, `csrf:{id}` | **Lost:** all active sessions; users **logged out** everywhere. |
| **Portfolios & watchlists** | `portfolio:{user}:{id}`, `watchlist:{user}:{id}` | **Lost:** user-created data unless restored. |
| **Price alerts & notifications** | `alert:{user}:{id}`, `notification:{user}:{id}` | **Lost:** alert definitions and in-app notifications. |
| **Feedback** | `feedback:{unix_bucket}` | **Lost:** submitted feedback blobs. |
| **Explorer RPC cache** | `address:…`, `tx:…`, `block:…`, `network:status`, `latest_blocks`, `latest_transactions` | **Recoverable:** repopulates from **blockchain RPC** and background jobs as users browse (no user data). |
| **Rates / pricing cache** | `rates:btc`, `btc_price_history:{currency}` | **Recoverable:** refetched from **pricing** integrations. |
| **News cache** | Provider-scoped keys from [`news` cache](../internal/news/) | **Recoverable:** refetched from **news API** (subject to quotas). |
| **Rate limits** | `rate:*`, `rate:ip:*`, `rate:user:*`, `export:heavy:*`, … | **Lost:** counters reset (usually acceptable). |
| **Export idempotency** | `idempotency:v1:{hash}` | **Lost:** prior export replay metadata; clients may **repeat exports** (see [`IDEMPOTENCY_KEYS.md`](IDEMPOTENCY_KEYS.md)). |
| **Admin default user** | `user:{ADMIN_USERNAME}` (if initialized) | **Lost:** until **`initializeDefaultAdmin`** runs again on startup ([`init.go`](../internal/server/init.go)) or you restore the key. |

**Not in Redis (process-local):** In-memory **`users` map** sync limitations across replicas are documented in [`HORIZONTAL_SCALING.md`](HORIZONTAL_SCALING.md). **Price-alert SCAN cursor** state is **per process** (not Redis); losing Redis does not “restore” that cursor—it resets on process restart anyway.

---

## 5. Total loss: “what user data is gone?”

If **all Redis data is permanently lost** and there is **no** backup, **no** export to another system, and **no** Postgres migration in use:

- **Gone:** registered **accounts**, **sessions**, **portfolios**, **watchlists**, **price alerts**, **notification history**, **feedback**, and **cached idempotency** state for exports.
- **Not gone (outside Redis):** **Blockchain** truth on the public network; **application binaries** and **config** in your image/repo; **secrets** in your secret manager (unless co-lost).
- **Recoverable by design (cache-only keys):** Explorer **address/tx/block** caches, **network status**, **rates**, **news** caches—at the cost of **cold start** latency and upstream load until caches warm.

**Business impact:** Treat total Redis loss **without backup** as **catastrophic** for **user-owned data** on this stack. RPO/RTO should be defined with stakeholders ([`SLO_AND_ERROR_BUDGET.md`](SLO_AND_ERROR_BUDGET.md)).

---

## 6. Testing restores

For **ad-hoc** verification: restore a **non-production** snapshot to a **scratch** Redis, point a **staging** build at it, and run smoke tests (login if users exist, list portfolios, one search).

For a **formal quarterly process** (simulated wipe, restore, validation checklist, written record), follow [**DISASTER_RECOVERY_DRILL.md**](DISASTER_RECOVERY_DRILL.md) ([ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **60**).

---

## Related documents

- [`DISASTER_RECOVERY_DRILL.md`](DISASTER_RECOVERY_DRILL.md) — quarterly simulated Redis wipe + restore drill.
- [`SQL_AND_REDIS_SAFETY.md`](SQL_AND_REDIS_SAFETY.md) — key layout and cluster caveats.
- [`HORIZONTAL_SCALING.md`](HORIZONTAL_SCALING.md) — shared Redis across replicas.
- [`POSTGRES_MIGRATION_SKETCH.md`](POSTGRES_MIGRATION_SKETCH.md) — future durable user store off Redis.
- [`HEALTH_AND_READINESS.md`](HEALTH_AND_READINESS.md) — readiness depends on Redis `PING`.
