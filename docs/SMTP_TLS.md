# SMTP TLS

Outbound email uses [`internal/email/smtp_sender.go`](../internal/email/smtp_sender.go): plain TCP to `SMTP_HOST`:`SMTP_PORT`, optional **STARTTLS** (`SMTP_STARTTLS=true`), then SMTP auth and delivery.

## Production and staging

- Set **`SMTP_STARTTLS=true`** for port **587** (or follow your provider’s documented TLS mode).
- **Do not** set `SMTP_SKIP_VERIFY=true`. The server refuses to start if `SMTP_SKIP_VERIFY` is set while **`APP_ENV` is not `development`** ([`Config.Validate`](../internal/config/config.go)).
- Use a hostname that matches the certificate presented by the mail server (typical for commercial SMTP: **CA-issued** certs verified against the system trust store).

## Development only (`SMTP_SKIP_VERIFY`)

For **local labs** with **self-signed** certificates or a corporate MITM proxy, you may set:

- `APP_ENV=development`
- `SMTP_SKIP_VERIFY=true`

`Validate` allows this combination only when `APP_ENV` is **development** (case-insensitive). On startup, the app logs a **high-visibility `WARN`** with `security_event=smtp_tls_verification_disabled` so operators cannot miss that TLS verification is off.

Never enable `SMTP_SKIP_VERIFY` in production, staging, or any environment where email confidentiality or integrity matters.

## Environment variables (reference)

| Variable | Role |
|----------|------|
| `SMTP_HOST`, `SMTP_PORT` | Mail server address |
| `SMTP_STARTTLS` | When `true`, upgrade the connection with TLS before auth |
| `SMTP_SKIP_VERIFY` | When `true`, **skip** TLS certificate verification (dev-only; see above) |

See also [`.env.example`](../.env.example) and the threat model ([`THREAT_MODEL.md`](THREAT_MODEL.md)).
