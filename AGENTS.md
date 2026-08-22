# OpenOutreach — agent context

Self-hosted, agent-first cold email platform. Fork of [andersmyrmel/cold-cli](https://github.com/andersmyrmel/cold-cli) plus hosted control plane, Gmail OAuth, Cloudflare Worker/Container, dashboard, tracking, and MCP.

Humans use the dashboard. Agents use MCP/API. Both drive the same engine:

**contacts → sequence → schedule → send → thread → reply → suppress → analytics**

## Product invariants (do not break)

1. **Create ≠ send.** Campaign create/draft never sends. Activate is explicit and consequential (`confirm: true` on API/MCP).
2. **Eager scheduling.** All sends live in `scheduled_sends`. Do not invent lazy `next_send_at` on campaign_leads.
3. **GWSClient is the mail provider.** Hosted Gmail is `GoogleAPIProvider` implementing `GWSClient`. Do not add a parallel MailProvider stack.
4. **Tick lock is caller-owned.** `AcquireTickLock` before `engine.Tick`. Never send when locked.
5. **Tick lock is session-safe.** Postgres: direct URL only (no Hyperdrive / transaction-mode PgBouncer). D1: `tick_lock` row. Never send when locked.
6. **Workspace via config/header.** `workspace_id` from `OPENOUTREACH_WORKSPACE_ID` / `X-Workspace-ID` — never email-domain inference.
7. **Thread continuity.** One account per lead for a campaign; step-1 backfills `thread_id` / `parent_message_id`.
8. **Error isolation.** One failed send marks that row `failed` and continues the tick.
9. **Open tracking is optional and approximate.** Label UI as **Approx. opens**. Never block send on tracking.
10. **Compose around `pkg/engine`.** Prefer thin re-exports over rewriting `internal/`.

## Layout

```
cmd/cold-cli/       upstream CLI
cmd/outreachd/      hosted HTTP (API + tick + OAuth + health)
internal/           upstream engine (minimal hosted hooks only)
internal/hosted/    Google provider, vault, tracking, HTTP API
pkg/engine/         public facade for hosted/API
worker/             Cloudflare Worker: proxy, cron, /t/*, MCP
web/                Vite + React ops dashboard
migrations/         additive hosted SQL
docs/               UPSTREAM, ARCHITECTURE, DEPLOYMENT, OAUTH, MCP, SECURITY, RELEASING
```

## Upstream hooks (keep diffs small)

| Hook | Why |
|------|-----|
| `TickConfig.MaxSendsPerTick` | Hosted cron / HTTP timeouts (use `1`) |
| `EmailParams.TrackingPixelURL` | Open pixel without forking send |
| `pkg/engine` re-exports | Hosted API without deep `internal` coupling |
| Hosted tables + `campaigns.open_tracking` via KV/bootstrap | credentials, tokens, classifications |

Settled cold-cli decisions (scheduler, templates as `ReplaceAll`, daily limits from events, `skipped` vs `cancelled`) live in upstream docs/`ARCHITECTURE.md` — do not revisit without explicit instruction.

## Hosted runtime

- **Listen:** `:8080` — `GET /internal/health`, `POST /internal/tick`
- **API:** `/api/v1/*` JSON envelope `{ data, error, warnings }` + `next_actions` where useful
- **Tick:** lock → `NoSleep=true`, `MaxSendsPerTick=1` → reply poll still every tick
- **Storage:** D1 (default, `/internal/d1`) or direct Postgres
- **OAuth scopes:** `openid` `email` `gmail.send` `gmail.readonly`
- **Mock:** `OPENOUTREACH_MOCK_GMAIL=1`
- **Worker cron:** `*/2 * * * *` UTC — cron ≠ send permission
- **Public paths:** `/t/o/*`, `/t/c/*`, Gmail OAuth callback. Auth: `AUTH_MODE=cloudflare_access` (default), `hosted` (Better Auth `/sign-in` + `/api/auth/*`), or `local_noauth`. MCP bearer still works.

## Dashboard UI direction

Target aesthetic (ops console, not marketing SaaS): **clean shadcn-like** — light surfaces, soft mint/teal accent, generous whitespace, soft rounding (~8–10px), thin icons, restrained hierarchy.

Visual reference: [docs/design/reference-aesthetic.png](docs/design/reference-aesthetic.png).

Do **not** default to purple gradients, dark-first chrome, pill clusters, or dense newspaper layouts. Prefer Inter / system geometric sans over display serifs.

Nav IA: Overview, Campaigns, Inbox, Leads, Sending Accounts, Settings.

## Testing

```bash
go test ./internal/hosted/ ./pkg/engine/ ./internal/ ./cmd/...
cd web && npm run build
cd worker && npm install --legacy-peer-deps && npx tsc --noEmit
```

- Real SQLite `:memory:` for behavioral tests; mock only `GWSClient`
- Every hosted codepath: happy path + key error branches
- Activate without `confirm` must fail

## Build & local run

```bash
go build -o outreachd ./cmd/outreachd
go build -o cold-cli ./cmd/cold-cli
docker compose up --build   # Postgres :5433, outreachd :8080, Vite :5173
```

## Agent / OSS norms

- Smallest correct change; prefer delete over abstract.
- Atomic commits; do not rewrite engine semantics for convenience.
- Never commit secrets (`.env`, keys, tokens).
- Attribute upstream cold-cli (MIT) in LICENSE/NOTICE.
- Releases: see [docs/RELEASING.md](docs/RELEASING.md) — semver tags `vX.Y.Z` on `main`.

## scheduled_sends statuses

```
pending → waiting
sent → gws/API success
failed → send error (error_message + events)
skipped → auto-cancel (reply/bounce/domain-reply)
cancelled → user pause/blacklist
```
