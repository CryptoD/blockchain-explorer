# Chaos testing (optional): Redis flakiness with Toxiproxy

Use [Toxiproxy](https://github.com/Shopify/toxiproxy) in **staging** or locally to inject latency, timeouts, and connection resets between the app and Redis—without changing application code beyond pointing at the proxy.

## Stack

[`scripts/chaos/docker-compose.yml`](../scripts/chaos/docker-compose.yml) runs:

- **redis** — real Redis (internal network only).
- **toxiproxy** — TCP proxy; **listen** `0.0.0.0:6379` inside the container, **upstream** `redis:6379`.

Published ports:

| Port | Purpose |
|------|--------|
| **8474** | Toxiproxy HTTP API (CLI + `curl`) |
| **6380** | Proxied Redis mapped to the host (`127.0.0.1:6380` → container `6379`) |

## Configure the application

The server reads **`REDIS_HOST`** and **`REDIS_PORT`** (default `6379`). Examples:

**App on the Docker host** (binary or `go run`), Redis through published port:

```bash
export REDIS_HOST=127.0.0.1
export REDIS_PORT=6380
```

**App in another Compose service** on the same `chaos` network:

```yaml
environment:
  - REDIS_HOST=toxiproxy
  - REDIS_PORT=6379
```

## Add toxics (examples)

Install the [CLI](https://github.com/Shopify/toxiproxy/releases) on your machine (it talks to `localhost:8474` by default), or run it inside the toxiproxy container:

```bash
docker compose -f scripts/chaos/docker-compose.yml exec -T toxiproxy \
  toxiproxy-cli toxic add -t latency -a latency=500 -a jitter=100 redis
```

With the CLI on the host (after `docker compose ... up -d`):

```bash
toxiproxy-cli toxic add -t latency -a latency=500 -a jitter=100 redis
```

Other useful toxics:

| Goal | Example |
|------|--------|
| Slow responses | `latency` (ms) + optional `jitter` |
| Hung connections | `timeout` |
| TCP reset | `reset_peer` |
| Drop service | `toxiproxy-cli toggle redis` (flip **enabled**) or `POST /proxies/redis` with `"enabled": false` |

Remove toxics or reset everything:

```bash
toxiproxy-cli toxic remove -n latency_downstream redis
toxiproxy-cli toggle redis
```

## Integration tests

For `BLOCKCHAIN_EXPLORER_TEST_REDIS=integration`, set **`TEST_REDIS_ADDR`** to a full `host:port`, or set **`REDIS_HOST`** + **`REDIS_PORT`** (see [`internal/redistest`](../internal/redistest/redistest.go)).

## Safety

Run chaos only in **non-production** environments. Toxiproxy can make Redis appear down or extremely slow; confirm blast radius (sessions, rate limits, cache) before wide use.
