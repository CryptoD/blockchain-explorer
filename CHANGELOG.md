# Changelog

All notable changes to this project are documented in this file. The project uses a single [Semantic Versioning](https://semver.org/) number for the **application and the `/api/v1` HTTP contract** (see [README](README.md) API section and [docs/API_VERSIONING.md](docs/API_VERSIONING.md)).

## [Unreleased]

### Documentation

- Aligned CSRF/session, threat model, bounded contexts, Redis key safety, internal package inventory, `SECURITY.md`, and `readme.md` with **machine API keys** (`Bearer bkx_*`).

## [1.1.0] - 2026-05-06

### Added

- **Machine API keys** (`Authorization: Bearer bkx_*`) for automation: per-user keys with `user:read` / `user:write` scopes, and admin-managed **service** keys with `admin:read` / `admin:write`. Storage in Redis (hashed secrets). Endpoints: `GET|POST /api/v1/user/api-keys`, `DELETE /api/v1/user/api-keys/{id}`, `GET|POST /api/v1/admin/api-keys`, `DELETE /api/v1/admin/api-keys/{id}`. See [docs/API_KEYS.md](docs/API_KEYS.md).

### Changed

- OpenAPI `info.version` aligned to **1.1.0**; admin status/cache and new key routes document optional bearer auth.

### Documentation

- Added this changelog at `CHANGELOG.md`.
