# Security

- Encrypt Google refresh/access tokens at rest (`CREDENTIAL_ENCRYPTION_KEY`).
- Never log tokens, SMTP passwords, encryption keys, or full secret headers.
- Tracking endpoints public; do not store raw client IPs (country via CF header is OK).
- Internal tick/track routes require `X-Internal-Token` when configured.
- Dashboard auth: `AUTH_MODE=cloudflare_access` (default) or `hosted` (Better Auth). Optional `AUTH_ALLOWED_EMAILS`.
- Workspace isolation: every campaign/account/credential query filters `workspace_id`.
- Activate is explicit; create never sends.
- Image open tracking is approximate and optional.

## Threat notes

- Concurrent cron: advisory lock → clean no-op.
- OAuth CSRF: single-use `state` rows.
- Container local disk is ephemeral; Postgres is source of truth.
