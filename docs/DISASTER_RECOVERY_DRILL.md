# Disaster recovery drill (quarterly Redis wipe + restore)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **60**. It defines a **recurring drill** to prove you can recover application state after a **total Redis loss** by **restoring from backup**, not just by reading the [backup runbook](REDIS_BACKUP_AND_RESTORE.md).

**Cadence:** at least **once per calendar quarter** in a **non-production** environment (or in production only with an **explicit change window** and leadership approval—usually unnecessary if staging matches backup/restore mechanics).

**Goal:** validate **RPO/RTO** assumptions, train operators, and surface gaps (missing backups, wrong retention, restore steps that do not match reality).

---

## 1. Preconditions

| Requirement | Why |
|-------------|-----|
| A **recent backup artifact** exists (managed snapshot, `dump.rdb` export, vendor point-in-time restore, etc.) | The drill is meaningless without a real object to restore. |
| **Staging** (or an isolated Redis + app stack) that may be **wiped** | Never use `FLUSHALL` / destructive restore on **production** for a drill unless policy explicitly allows it. |
| **Restore runbook** reviewed: [REDIS_BACKUP_AND_RESTORE.md](REDIS_BACKUP_AND_RESTORE.md) | Same Redis major version and persistence mode as production where possible. |
| **Smoke-test user** or seed script | Known username/password or API token to verify login after restore. |
| **Owner** for the drill (SRE/on-call + optional dev witness) | Accountability and notes. |

---

## 2. Simulated “Redis wipe”

Pick **one** approach (all model **empty data directory** / total key loss):

1. **Scratch instance (preferred):** provision a **new** empty Redis (new pod, new managed instance, or new Docker volume). Point the **staging** app at it—**no flush** needed; data volume is empty. Then **restore** the backup **into** this instance (vendor “restore to new” / copy RDB / clone).
2. **Destructive flush (staging only):** on a dedicated Redis that contains **no production traffic**, run `FLUSHALL` or stop Redis, **delete** the data files, start empty—then **restore** from backup. **Requires double-check** the hostname/port and environment.
3. **Managed cloud “restore to new cluster”** — treat the **pre-restore** empty endpoint as the “wipe”; follow the provider’s workflow.

**Do not** run destructive commands against a shared dev Redis others depend on without coordination.

---

## 3. Restore from backup

Follow §3 of [REDIS_BACKUP_AND_RESTORE.md](REDIS_BACKUP_AND_RESTORE.md) (restore outline), adapted to your platform:

1. Restore artifact to the **target** Redis until `INFO keyspace` shows **non-zero** keys (or expected subset if partial backup).
2. Align **app config** (`REDIS_HOST`, `REDIS_PORT`, password/TLS) to the restored instance.
3. Restart or roll app pods so connections pick up **DNS** if the endpoint changed.

Record **wall-clock time** from “restore started” to “app ready”—this is an **RTO** sample for the runbook.

---

## 4. Validation checklist (minimum)

Complete in order; capture **pass/fail** in the record (§6).

| Step | Check |
|------|--------|
| Redis | `redis-cli -u … PING` → `PONG` |
| Redis | `DBSIZE` or `INFO keyspace` — **not** zero if backup contained data |
| App | `GET /ready` (or `/readyz`) → **200** when Redis is wired ([`HEALTH_AND_READINESS.md`](HEALTH_AND_READINESS.md)) |
| App | Login with **smoke-test** user (session keys restored) or re-seed if backup predates user |
| App | **One read** each: profile, portfolio or watchlist list, explorer search (validates cache + user paths) |
| Optional | Spot-check keys: `user:*`, `session:*` prefixes per [`SQL_AND_REDIS_SAFETY.md`](SQL_AND_REDIS_SAFETY.md) / [`internal/repos/keys.go`](../internal/repos/keys.go) |

If login fails but keys exist, suspect **wrong backup**, **version mismatch**, or **partial restore**.

---

## 5. Failure modes to deliberately note

Use the drill to confirm behavior—not only happy path:

- Backup **older** than expected (RPO breach): document how much data was “lost” vs live staging.
- Restore to **wrong Redis version**: capture error messages for the runbook.
- **Secrets**: restored Redis ACL/password matches what the app uses.

---

## 6. Record template (per quarter)

File an internal ticket or wiki page with:

- **Date**, **environment** (e.g. staging-cluster-b)
- **Backup ID** / artifact path / timestamp
- **Wipe method** (new empty instance vs FLUSHALL)
- **Restore duration** (minutes)
- **Validation** table (§4) pass/fail
- **Issues found** (e.g. “restore doc missing TLS step”) and **follow-ups** (owners + due dates)
- **Participants**

---

## 7. What this drill does *not* replace

- **Application code** tests (`go test`, CI) — those use **miniredis** or ephemeral Redis; they do **not** validate real backup files or cloud restore APIs.
- **Multi-region failover** — if you rely on Redis replication across regions, test that separately.
- **Postgres** — not in scope for this app’s current primary store; see [POSTGRES_MIGRATION_SKETCH.md](POSTGRES_MIGRATION_SKETCH.md) if added later.

---

## Related documents

- [REDIS_BACKUP_AND_RESTORE.md](REDIS_BACKUP_AND_RESTORE.md) — RDB/AOF, backup/restore procedures, data lost on total loss.
- [SLO_AND_ERROR_BUDGET.md](SLO_AND_ERROR_BUDGET.md) — RPO/RTO discussion.
- [CHAOS_TESTING.md](CHAOS_TESTING.md) — optional network chaos; complements but does not replace backup restore.
