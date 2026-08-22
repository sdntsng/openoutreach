# Architecture

```text
Browser / MCP client
        │
        ▼
Cloudflare Worker  ── public /t/o /t/c, Gmail OAuth callback
        │              AUTH_MODE=cloudflare_access (default) or hosted Better Auth
        │ scheduled */2
        ▼
Cloudflare Container (outreachd :8080)
        │
        ├─ engine.Tick (lock → poll → send ≤1)
        ├─ REST /api/v1
        └─ Google OAuth + encrypted credentials
                │
                ▼
        Direct Postgres (advisory lock)  **or**  D1 via Worker /internal/d1 (tick_lock row)
```

## Components

- **cold-cli** — CLI + engine (`pkg/engine`)
- **outreachd** — hosted HTTP control plane in the Container
- **Worker** — edge gateway, cron, tracking pixels, MCP, static assets
- **web** — React ops dashboard

## Tick path

1. Cron fires Worker `scheduled()`.
2. Worker starts singleton Container `getContainer(OUTREACH, "default")`.
3. `POST /internal/tick` with `X-Internal-Token`.
4. outreachd `AcquireTickLock`; if held → `{status:"locked"}` 200.
5. `engine.Tick` with `NoSleep=true`, `MaxSendsPerTick=1`.
6. Spacing comes from eager `send_at` + cron, not in-request sleep.

## Providers

- `GWSCLI` — subprocess gws (CLI)
- `GoogleAPIProvider` — Gmail API + vault (hosted)
- `MockGmail` — local/dev
- SMTP/IMAP — unchanged upstream transports

## Auth

`AUTH_MODE` on the Worker: **`cloudflare_access`** (default, Zero Trust), **`hosted`** (Better Auth Google/email), or **`local_noauth`**. Tracking + Gmail OAuth callback public. MCP bearer or Access/session. Internal tick token for Worker→Container.
