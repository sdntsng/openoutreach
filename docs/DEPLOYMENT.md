# Deployment

OpenOutreach production runs on **Cloudflare Workers + Containers**. Storage is **Cloudflare D1 (default)** or **direct Postgres**. The dashboard ships as static assets on the Worker; the engine runs in a singleton Container (`outreachd`).

```text
Browser / MCP → Cloudflare Worker → Container (outreachd :8080)
                     │                    │
                     │ /internal/d1       ├─ D1 (default) or direct Postgres
                     ↑ cron */2 POST /internal/tick
                     ↑ /t/o, /t/c at edge
```

## Storage {#storage}

Pick one. Both keep tick locking so two ticks cannot send at once.

| Mode | When to use | Tick lock |
|------|-------------|-----------|
| **D1** (`STORAGE=d1`, default) | One-click CF-native, no external DB | `tick_lock` row in D1 |
| **Postgres** (`STORAGE=postgres`) | You already run Postgres, larger scale | `pg_try_advisory_lock` on a **direct** session |

### D1 {#d1}

The Worker holds a D1 binding (`DB`). The Container talks SQLite dialect through `POST /internal/d1` (internal token). Schema bootstrap is the existing SQLite DDL.

D1 statements auto-commit (no interactive SQL transactions across HTTP). Unique constraints still prevent duplicate leads/sends.

### Postgres {#postgres}

**Use a direct Postgres connection string.** Do **not** use PgBouncer **transaction** pooling or Cloudflare Hyperdrive on the tick / advisory-lock connection. Pooled transaction modes break lock semantics and can duplicate sends.

| Provider | Connection | Notes |
|----------|------------|-------|
| **Neon** | Direct host (not `*-pooler*`) | Free tier OK for dev |
| **Supabase** | Port `5432` / direct mode | Avoid transaction pooler on port `6543` |
| **Self-hosted** | Direct URL | Must allow outbound from Cloudflare Containers |

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| Cloudflare account | Workers + Containers (+ D1 for default storage) |
| Storage | D1 (created by deploy script) **or** direct Postgres |
| Google OAuth app | For real Gmail; optional until you connect accounts |
| `wrangler` login | `cd worker && npx wrangler login` |

## Deploy paths

### A. One-command script (recommended)

```bash
cp .env.deploy.example .env.deploy
# Default STORAGE=d1 — no DATABASE_URL needed
# For Postgres: STORAGE=postgres and set DATABASE_URL
./scripts/deploy-cf.sh
```

The script generates missing secrets, uploads them via `wrangler secret put`, builds the dashboard, and deploys Worker + Container.

Options: `--dry-run`, `--skip-build`, `--no-generate`, `--env-file PATH`.

### B. Deploy to Cloudflare button

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/sdntsng/openoutreach&dir=worker)

During setup:

- **Root directory:** `worker`
- **Build command:** `npm run build`
- **Deploy command:** `npm run deploy`
- Enter secrets when prompted (descriptions come from `worker/package.json` → `cloudflare.bindings`)

The button forks the repo and configures Workers Builds CI. Use the CLI script if you deploy from this repo without forking.

### C. Manual wrangler

```bash
cd worker
npm install --legacy-peer-deps
cd ../web && npm install && npm run build
cd ../worker

# Required secrets (see worker/.dev.vars.example)
echo -n "$DATABASE_URL" | npx wrangler secret put DATABASE_URL
echo -n "$CREDENTIAL_ENCRYPTION_KEY" | npx wrangler secret put CREDENTIAL_ENCRYPTION_KEY
echo -n "$INTERNAL_CONTAINER_TOKEN" | npx wrangler secret put INTERNAL_CONTAINER_TOKEN
echo -n "$MCP_BEARER_TOKEN" | npx wrangler secret put MCP_BEARER_TOKEN
echo -n "$TRACKING_HMAC_SECRET" | npx wrangler secret put TRACKING_HMAC_SECRET
echo -n "$PUBLIC_BASE_URL" | npx wrangler secret put PUBLIC_BASE_URL
# Optional for Gmail
npx wrangler secret put GOOGLE_CLIENT_ID
npx wrangler secret put GOOGLE_CLIENT_SECRET

npx wrangler deploy
```

Container receives runtime env forwarded from Worker secrets on start (`COLD_CLI_DATABASE_URL`, encryption key, Google client, internal token, `PUBLIC_BASE_URL`, etc.).

Cron: `*/2 * * * *` (UTC). Cron does not imply a send — spacing comes from `scheduled_sends.send_at`.

## Secrets reference

| Secret | Required | How to get |
|--------|----------|------------|
| `DATABASE_URL` | Postgres mode | Direct Postgres URL |
| `DB` (D1 binding) | D1 mode | Created as database `openoutreach` |
| `CREDENTIAL_ENCRYPTION_KEY` | Yes | `openssl rand -hex 32` — **do not rotate** after connecting Gmail |
| `INTERNAL_CONTAINER_TOKEN` | Yes | `openssl rand -hex 32` |
| `MCP_BEARER_TOKEN` | Yes | `openssl rand -hex 32` |
| `TRACKING_HMAC_SECRET` | Yes | `openssl rand -hex 32` |
| `PUBLIC_BASE_URL` | Yes | `https://openoutreach.<account>.workers.dev` or custom domain |
| `GOOGLE_CLIENT_ID` | For Gmail | Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | For Gmail | Google Cloud Console |
| `OPENOUTREACH_WORKSPACE_ID` | No | Default `default` (wrangler var) |
| `AUTH_MODE` | No | `cloudflare_access` (default), `hosted`, or `local_noauth` |
| `CF_ACCESS_AUD` | Access mode | Access application AUD tag (`POLICY_AUD` alias) |
| `BETTER_AUTH_SECRET` | Hosted mode | `openssl rand -hex 32` — dashboard sessions |
| `AUTH_ALLOWED_EMAILS` | No | Comma-separated allowlist (Access policy + Better Auth signup) |
| `GOOGLE_REDIRECT_URL` | No | Defaults to `{PUBLIC_BASE_URL}/api/v1/accounts/google/oauth/callback` |
| `MICROSOFT_CLIENT_ID` | For Outlook | Entra ID app registration |
| `MICROSOFT_CLIENT_SECRET` | For Outlook | Entra ID client secret |
| `MICROSOFT_TENANT_ID` | No | Default `common` |
| `FEATURE_CF_EMAIL` | No | `1` to enable Cloudflare Email Sending accounts (default off) |
| `FEATURE_RESEND` | No | `1` to enable Resend send-only accounts |

## Cloudflare Email Sending {#cf-email}

Cloudflare [Email Service](https://developers.cloudflare.com/email-service/) is a **transactional** API mailer (`GWSClient`), not a Gmail/M365 inbox. Weak for cold-inbox reputation vs Workspace mailboxes. Sending to arbitrary recipients requires **Workers Paid** (~3,000 emails/month included, then ~$0.35/1k).

1. Onboard the sending domain in the Cloudflare dashboard (**Compute & AI → Email Service**). Add the SPF/DKIM records it prints.
2. Create an API token with **Email Sending: Edit** (and Account read if the picker requires it).
3. Set `FEATURE_CF_EMAIL=1` (Worker secret or `.env.deploy`) and re-deploy so the container sees the flag.
4. Dashboard → Sending Accounts → **Add Cloudflare Email** (`from` address, Cloudflare **account id**, API token). Token is vaulted and never returned.
5. **Email Routing:** create a rule that sends mail for that from-address (or the domain) to **this Worker**. The Worker `email()` handler posts MIME to `POST /api/v1/integrations/cf-email/inbound` with `X-Internal-Token`. Do **not** mark that path Access-public; the handler never goes through the browser.
6. Do **not** add a wrangler `send_email` binding until Email Sending is onboarded — the default deploy has none. outreachd sends via REST (`POST /accounts/{account_id}/email/sending/send`).

SMTP alternative (still no IMAP): `smtp.mx.cloudflare.net:465`, username `api_token`, password = API token. Prefer REST + Routing so replies land in Inbox.

## Post-deploy checklist

1. **Set `PUBLIC_BASE_URL`** to your Worker URL (`https://openoutreach.<subdomain>.workers.dev`) or custom domain. Re-run `./scripts/deploy-cf.sh` if you update it.
2. **Google OAuth redirect URI** (Gmail sending accounts):
   `{PUBLIC_BASE_URL}/api/v1/accounts/google/oauth/callback`
   If `AUTH_MODE=hosted`, also add `{PUBLIC_BASE_URL}/api/auth/callback/google`.
   See [GOOGLE_OAUTH.md](GOOGLE_OAUTH.md).
3. **Auth (default: Cloudflare Access).** Run `./scripts/setup-cf-access.sh` (needs `CLOUDFLARE_API_TOKEN` with **Access: Apps and Policies Write** — `wrangler login` OAuth cannot create Access apps). Or create apps in Zero Trust dashboard: allow `AUTH_ALLOWED_EMAILS`, set `CF_ACCESS_AUD`, bypass `/t/*`, `/internal/*`, and the Gmail OAuth callback. Or set `AUTH_MODE=hosted` for in-app Google/email sign-in.
4. Dashboard → **Settings → Sending Accounts → Connect Google**. Cloudflare Email: see [Cloudflare Email Sending](#cf-email) (`FEATURE_CF_EMAIL=1`).

## Verify

```bash
export PUBLIC_BASE_URL=https://openoutreach.<account>.workers.dev
export INTERNAL_CONTAINER_TOKEN=<from .env.deploy>

curl -sS "$PUBLIC_BASE_URL/internal/health" \
  -H "X-Internal-Token: $INTERNAL_CONTAINER_TOKEN" | jq
```

Create a draft campaign (does not send), preview, then activate with `confirm: true`. See [README.md](../README.md) quickstart for curl examples (swap `localhost:8080` for `PUBLIC_BASE_URL`).

## Dashboard auth

Same three modes as OpenSEO self-host / hosted. Set `AUTH_MODE` on the Worker (default **`cloudflare_access`**).

| Mode | Who it's for | What happens |
|------|----------------|--------------|
| `cloudflare_access` | Default. Single-tenant CF deploy | Cloudflare Access login (Google/email via Zero Trust). Set `CF_ACCESS_AUD`. |
| `hosted` | In-app accounts | Better Auth: `/sign-in` with Google + email/password. Needs `BETTER_AUTH_SECRET` + D1 auth tables. |
| `local_noauth` | Docker / private network | No dashboard login. Do not expose to the internet. |

### Cloudflare Access (default)

Protect `/`, `/api/*`, `/mcp/*` at the Access application. Bypass `/t/*`, `/internal/*` (Worker still requires `X-Internal-Token`), `{PUBLIC_BASE_URL}/api/v1/accounts/google/oauth/callback`, `{PUBLIC_BASE_URL}/api/v1/accounts/microsoft/oauth/callback`, and Clay/generic ingest `POST /api/v1/integrations/{provider}/ingest`.

CLI (recommended for `openoutreach.siddhant.site` or your custom hostname):

```bash
# Create token: dash.cloudflare.com → My Profile → API Tokens
# Permission: Account → Access: Apps and Policies → Edit

export CLOUDFLARE_API_TOKEN=...
export PUBLIC_BASE_URL=https://openoutreach.siddhant.site
export AUTH_ALLOWED_EMAILS=you@example.com   # optional; default from wrangler vars

chmod +x scripts/setup-cf-access.sh
./scripts/setup-cf-access.sh
./scripts/deploy-cf.sh --skip-build   # refresh container PUBLIC_BASE_URL
```

The script creates four self-hosted Access apps (main Allow + three Bypass paths), writes `CF_ACCESS_AUD` to `.env.deploy`, patches `worker/wrangler.jsonc`, and runs `wrangler secret put CF_ACCESS_AUD`.

When `CF_ACCESS_AUD` is set, the Worker also rejects requests that lack `Cf-Access-Jwt-Assertion` (except public paths). MCP clients can use `Authorization: Bearer $MCP_BEARER_TOKEN` instead of Access service tokens.

### Better Auth (`AUTH_MODE=hosted`)

- `/sign-in`, `/sign-up`, `/api/auth/*` are public
- Dashboard and `/api/v1/*` require a session cookie
- Optional `AUTH_ALLOWED_EMAILS` makes signup invite-only
- Google needs `{PUBLIC_BASE_URL}/api/auth/callback/google` on the OAuth client

## Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| Deploy fails on missing secrets | Run `./scripts/deploy-cf.sh` or set all `secrets.required` in wrangler |
| Health check 502 / timeout | Container cold start (~10–30s after idle); retry |
| Tick returns `{status:"locked"}` | Normal — another tick holds the advisory lock |
| OAuth redirect mismatch | Fix Google console URI + `PUBLIC_BASE_URL` |
| DB connection errors | Pooler URL, firewall blocking CF Containers, wrong credentials |
| 401 on dashboard/API | Not signed in, or Access JWT / MCP bearer missing |
| 503 Access not configured | `AUTH_MODE=cloudflare_access` without `CF_ACCESS_AUD` |
| `GOOGLE_CLIENT_ID/SECRET not set` on Connect Google | Container missing creds — re-run `./scripts/deploy-cf.sh` (writes `worker/src/container-env.ts`); confirm redirect URI `{PUBLIC_BASE_URL}/api/v1/accounts/google/oauth/callback` in Google Console |
| Duplicate sends | Transaction pooler on Postgres — switch to direct connection |

## Local compose

```bash
docker compose up --build
```

Postgres on `localhost:5433`, outreachd on `:8080`, Vite on `:5173`. Local stack does not deploy to Cloudflare.
