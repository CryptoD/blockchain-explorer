# Deployment guide

Production deployment paths for **Bitcoin Blockchain Explorer**:

| Path | Use when |
|------|----------|
| **[Helm chart](helm/blockchain-explorer/)** | Kubernetes (recommended) |
| **[docker-compose.prod.yml](../docker-compose.prod.yml)** | Single host / VM |
| **[k8s/](../k8s/)** | Legacy raw manifests (prefer Helm for new installs) |

The Go application listens on **`:8080`** by default (`APP_PORT` / `HTTP_LISTEN_ADDR`). Persistence is **Redis only** — no Postgres required despite legacy compose/k8s Postgres snippets.

---

## Prerequisites

- Built container image (`docker build -t blockchain-explorer:latest .`) or a registry image reference
- Redis reachable from the app (managed Redis recommended for production)
- Secrets: `GETBLOCK_ACCESS_TOKEN`, `ADMIN_PASSWORD` (required outside `APP_ENV=development`)
- See [`.env.example`](../.env.example) and [`internal/config/config.go`](../internal/config/config.go) for the full env reference

---

## Helm (Kubernetes)

### Install

```bash
# Create namespace and secrets (never commit real values)
kubectl create namespace blockchain-explorer

kubectl create secret generic blockchain-explorer-secrets \
  --namespace blockchain-explorer \
  --from-literal=GETBLOCK_ACCESS_TOKEN='your-token' \
  --from-literal=ADMIN_PASSWORD='your-strong-password'

# Lint and render
helm lint deploy/helm/blockchain-explorer

helm template blockchain-explorer deploy/helm/blockchain-explorer \
  --namespace blockchain-explorer \
  -f deploy/helm/blockchain-explorer/values.yaml \
  --set existingSecret=blockchain-explorer-secrets \
  --set redis.external.host=your-redis.example.com \
  --set config.getblockBaseUrl=https://go.getblock.io/your-project-id \
  | kubectl apply --dry-run=client -f -

# Install
helm upgrade --install blockchain-explorer deploy/helm/blockchain-explorer \
  --namespace blockchain-explorer \
  --set existingSecret=blockchain-explorer-secrets \
  --set redis.external.host=your-redis.example.com \
  --set config.getblockBaseUrl=https://go.getblock.io/your-project-id
```

### Production values

Create a private values file (e.g. `values-prod.yaml`):

```yaml
replicaCount: 2
image:
  repository: ghcr.io/your-org/blockchain-explorer
  tag: v1.1.0
  # digest: sha256:...   # preferred for immutable deploys

existingSecret: blockchain-explorer-secrets

redis:
  external:
    enabled: true
    host: your-managed-redis.example.com
    port: 6379
  deployInCluster:
    enabled: false   # demo/dev only; pin redis.deployInCluster.image if enabled

ingress:
  enabled: true
  hosts:
    - host: explorer.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: explorer-tls
      hosts:
        - explorer.example.com

config:
  getblockBaseUrl: https://go.getblock.io/your-project-id
  appBaseUrl: https://explorer.example.com
```

Apply cert-manager ClusterIssuer separately if using TLS ([`k8s/cluster-issuer.yaml`](../k8s/cluster-issuer.yaml)).

### Upgrade / rollback

```bash
helm upgrade blockchain-explorer deploy/helm/blockchain-explorer \
  --namespace blockchain-explorer \
  -f values-prod.yaml

helm rollback blockchain-explorer --namespace blockchain-explorer
```

### Verify

```bash
kubectl -n blockchain-explorer get pods
kubectl -n blockchain-explorer port-forward svc/blockchain-explorer 8080:8080
curl -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
```

---

## Docker Compose (single host)

```bash
cp .env.example .env
# Edit .env: GETBLOCK_*, ADMIN_PASSWORD, etc.

docker build -t blockchain-explorer:latest .

docker compose -f docker-compose.prod.yml config   # validate
docker compose -f docker-compose.prod.yml up -d

curl -fsS http://localhost:8080/health
```

Notes:

- **Postgres/Adminer** from dev [`docker-compose.yml`](../docker-compose.yml) are not included — the app does not use SQL.
- The prod overlay sets `read_only: true` with a `tmpfs` at `/tmp` (coordinate with task 82 distroless).
- App container healthcheck is commented out until the runtime image includes `wget`/`curl` (distroless uses orchestrator HTTP probes only — see task 82).

---

## Legacy k8s manifests

[`k8s/deployment.yaml`](../k8s/deployment.yaml) — app + Redis on port **8080** (fixed from legacy 3000).

[`k8s/legacy-postgres.yaml`](../k8s/legacy-postgres.yaml) — unused Postgres resources, kept for reference only.

[`k8s/ingress.yaml`](../k8s/ingress.yaml) — example Ingress (service port **8080**).

---

## Security

- Never commit secrets in values files or manifests — use `existingSecret` / `secretKeyRef` / env files.
- Set `securityContext` (non-root, read-only root FS, drop ALL caps) — enabled in Helm defaults; coordinate hardening with task 82.
- See [docs/HORIZONTAL_SCALING.md](../docs/HORIZONTAL_SCALING.md) and [docs/REDIS_BACKUP_AND_RESTORE.md](../docs/REDIS_BACKUP_AND_RESTORE.md).
