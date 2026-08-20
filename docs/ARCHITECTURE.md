# Architecture

```text
Browser / MCP client
        │
        ▼
Cloudflare Worker  ── public /t/o /t/c
        │              Access-protected /api /mcp /dashboard
        │ scheduled */2
        ▼
Cloudflare Container (outreachd :8080)
        │
        ├─ engine.Tick (lock → poll → send ≤1)
        ├─ REST /api/v1
        └─ Google OAuth + encrypted credentials
                │
                ▼
        Direct Postgres (session / advisory locks)
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

Cloudflare Access on dashboard/API/MCP. Tracking + OAuth callback public. Internal tick token for Worker→Container. Worker verifies Access JWT signature when `CF_ACCESS_AUD` is set.

## Tracking

- **Open tracking:** optional per campaign; Worker serves `/t/o/{token}` pixel, records via container internal API. Label UI as **Approx. opens**.
- **Click tracking:** Worker route and record handler exist (`/t/c/*`), but link rewriting at send time is not wired in V1 — clicks are not tracked in outbound emails yet.
