# Postgres migration sketch (users and durable state off Redis)

This document satisfies [ROADMAP_TO_100.md](../ROADMAP_TO_100.md) task **50**. It is a **design sketch only**—the application today persists accounts, sessions, portfolios, watchlists, alerts, notifications, and feedback primarily in **Redis** (see [`internal/repos/keys.go`](../internal/repos/keys.go) and [`docs/SQL_AND_REDIS_SAFETY.md`](SQL_AND_REDIS_SAFETY.md)). Nothing here is implemented or required for current deployments.

## Goals of a future migration

1. **Durable relational storage** for multi-row, query-heavy, or compliance-sensitive data (users, audit-friendly feedback, reporting on portfolios).
2. **Optional retention**: easier TTL/archival policies for notifications and sessions than ad hoc Redis scans.
3. **Keep Redis** (or another cache) for **ephemeral / high-churn** data: explorer RPC cache (`address:`, `tx:`, `block:`), rate-limit counters, optional session cache layer, `rates:btc`, chart aggregates—unless you explicitly move those too.

## Design principles

- **Surrogate keys** in Postgres (`bigserial` / `uuid`) with a stable **`external_id`** column where the app today uses string IDs (portfolio id, watchlist id, alert id), to avoid changing API contracts in one big bang.
- **Passwords**: store only **argon2id** or **bcrypt** hashes; never migrate plaintext.
- **Sessions**: store **hashed** refresh/session secrets if you move from opaque cookie → server-side row; align TTL with current **24h** session behavior ([`SessionRepo`](../internal/repos/session.go)).
- **CSRF**: can live as columns on `sessions` or a small `session_csrf` table with the same TTL as the session.
- **Arrays** (`news_sources_favorite`, `news_sources_blocked`, tags): **`jsonb`** is acceptable for a first schema; normalize to junction tables if you need constraints or search.

## Entity–relationship (conceptual)

```text
users 1──* portfolios
users 1──* portfolio_items (via portfolios)
users 1──* watchlists
users 1──* watchlist_entries (via watchlists)
users 1──* price_alerts
users 1──* notifications
users 1──* sessions (and csrf)
feedback is anonymous (no FK to users) or optional user_id later
```

## SQL sketch (Postgres 15+)

```sql
-- Extensions (optional)
-- CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id                bigserial PRIMARY KEY,
    username          text NOT NULL UNIQUE,
    password_hash     text NOT NULL,
    role              text NOT NULL CHECK (role IN ('user', 'admin')),
    email             text,
    preferred_currency text,
    theme             text,
    language          text,
    notifications_email            boolean NOT NULL DEFAULT false,
    notifications_price_alerts     boolean NOT NULL DEFAULT false,
    email_price_alerts             boolean NOT NULL DEFAULT false,
    email_portfolio_events         boolean NOT NULL DEFAULT false,
    email_product_updates          boolean NOT NULL DEFAULT false,
    default_landing_page           text,
    news_sources_favorite          jsonb NOT NULL DEFAULT '[]',
    news_sources_blocked           jsonb NOT NULL DEFAULT '[]',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_username_lower ON users (lower(username));

-- Server-side session + CSRF (replaces session:{id} and csrf:{id} in Redis)
CREATE TABLE sessions (
    id              text PRIMARY KEY,  -- opaque id from cookie
    user_id         bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    csrf_token_hash bytea NOT NULL,    -- HMAC-SHA256 or bcrypt of token
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

CREATE TABLE portfolios (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id     text NOT NULL,     -- current Redis portfolio id string
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (user_id, external_id)
);

CREATE TABLE portfolio_items (
    id              bigserial PRIMARY KEY,
    portfolio_id    bigint NOT NULL REFERENCES portfolios (id) ON DELETE CASCADE,
    position        int NOT NULL DEFAULT 0,
    type            text NOT NULL,
    address         text NOT NULL DEFAULT '',
    label           text NOT NULL DEFAULT '',
    amount          double precision NOT NULL,
    symbol          text NOT NULL DEFAULT '',
    UNIQUE (portfolio_id, position)
);

CREATE INDEX idx_portfolio_items_portfolio ON portfolio_items (portfolio_id);

CREATE TABLE watchlists (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id     text NOT NULL,
    name            text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (user_id, external_id)
);

CREATE TABLE watchlist_entries (
    id              bigserial PRIMARY KEY,
    watchlist_id    bigint NOT NULL REFERENCES watchlists (id) ON DELETE CASCADE,
    position        int NOT NULL DEFAULT 0,
    entry_type      text NOT NULL CHECK (entry_type IN ('symbol', 'address')),
    symbol          text NOT NULL DEFAULT '',
    address         text NOT NULL DEFAULT '',
    tags            jsonb NOT NULL DEFAULT '[]',
    notes           text NOT NULL DEFAULT '',
    group_label     text NOT NULL DEFAULT '',
    UNIQUE (watchlist_id, position)
);

CREATE TABLE price_alerts (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id     text NOT NULL,
    symbol          text NOT NULL,
    currency        text NOT NULL,
    threshold       double precision NOT NULL,
    direction       text NOT NULL CHECK (direction IN ('above', 'below')),
    delivery_method text NOT NULL,
    is_active       boolean NOT NULL DEFAULT true,
    triggered_at    timestamptz,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (user_id, external_id)
);

CREATE INDEX idx_price_alerts_user_active ON price_alerts (user_id) WHERE is_active AND triggered_at IS NULL;

CREATE TABLE notifications (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id     text NOT NULL,
    type            text NOT NULL,
    title           text NOT NULL,
    message         text NOT NULL,
    read_at         timestamptz,
    dismissed_at    timestamptz,
    created_at      timestamptz NOT NULL,
    UNIQUE (user_id, external_id)
);

CREATE INDEX idx_notifications_user_created ON notifications (user_id, created_at DESC);

-- Anonymous feedback (matches time-bucket Redis keys conceptually; use a single table + created_at)
CREATE TABLE feedback (
    id              bigserial PRIMARY KEY,
    created_at      timestamptz NOT NULL DEFAULT now(),
    name            text,
    email           text,
    message         text NOT NULL,
    client_ip       inet
);

CREATE INDEX idx_feedback_created_at ON feedback (created_at DESC);
```

## What would likely stay in Redis (initial hybrid)

| Area | Reason |
|------|--------|
| Block / tx / address JSON cache | High churn, TTL-friendly, not relational |
| `rates:btc`, mempool/chart keys | Short TTL, simple KV |
| Rate limit counters | Very high write rate; Redis strings + INCR fit well |
| Optional: second-level session cache | Read-through cache in front of Postgres sessions |

## Migration phases (high level)

1. **Introduce Postgres** alongside Redis; add `DATABASE_URL`, driver, and repository interfaces (similar to existing [`service_interfaces.go`](../internal/server/service_interfaces.go) pattern).
2. **Dual-write** new signups / updates to Postgres + Redis, or **Postgres primary + Redis read cache**, behind feature flags.
3. **Backfill** existing Redis keys into Postgres (batch jobs per `user:*`, `portfolio:*`, etc.); verify row counts and spot-check payloads.
4. **Cut reads** to Postgres per domain (users → portfolios → …); disable Redis writes for that domain.
5. **Remove** obsolete Redis keys or leave a short TTL mirror for rollback.

## Operational notes

- Run **`updated_at`** triggers or set in application code on every mutation.
- **Connection pooling**: PgBouncer or pooler in transaction mode for serverless; tune `max_conns` for Gin.
- **Backups and PITR**: required once users are in Postgres; Redis snapshots alone are no longer sufficient for account recovery.

## Related documents

- [SQL_AND_REDIS_SAFETY.md](SQL_AND_REDIS_SAFETY.md) — current Redis key layout and “no SQL yet” audit.
- [THREAT_MODEL.md](THREAT_MODEL.md) — user-data assets and trust boundaries.
