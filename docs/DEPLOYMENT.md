# Deployment

## Critical: Postgres for tick

**Use a direct Postgres connection string for outreachd / Container.**

Do **not** use:

- PgBouncer **transaction** pooling
- Cloudflare Hyperdrive for the tick / advisory-lock connection

`cold-cli` uses `pg_try_advisory_lock` on a dedicated session connection. Pooled transaction modes break lock semantics and can duplicate sends.

**Provider notes:**

- **Neon:** use the direct host (`*.neon.tech`), not the `-pooler` endpoint.
- **Supabase:** use the session pooler on port `5432`, not the transaction pooler on `6543`.

Worker tracking inserts may use Hyperdrive later; V1 routes tracking through the Container.

## Prerequisites

1. **Cloudflare Workers Paid** plan ($5/mo minimum) — Containers are not available on the free tier.
2. **Custom domain** on a zone in your Cloudflare account (e.g. `outreach.example.com`). Cloudflare Access cannot protect `*.workers.dev`; production disables `workers.dev` and serves only on the custom domain.
3. **Direct Postgres** DSN (see above).
4. **Google OAuth client** from a GCP project in your Workspace org with **Internal** consent screen (see [GOOGLE_OAUTH.md](GOOGLE_OAUTH.md)).

## GitHub Actions deploy (recommended)

Configure repository **secrets**:

| Secret | Purpose |
|--------|---------|
| `CLOUDFLARE_API_TOKEN` | Workers Scripts Edit + Containers + R2 (image registry) |
| `CLOUDFLARE_ACCOUNT_ID` | Your Cloudflare account ID |

Configure repository **variable**:

| Variable | Example | Purpose |
|----------|---------|---------|
| `OPENOUTREACH_HOSTNAME` | `outreach.example.com` | Custom domain route + `PUBLIC_BASE_URL` |

Set Worker **secrets** once (per environment):

```bash
cd worker
echo -n "$DATABASE_URL" | npx wrangler secret put DATABASE_URL --env production
npx wrangler secret put GOOGLE_CLIENT_ID --env production
npx wrangler secret put GOOGLE_CLIENT_SECRET --env production
npx wrangler secret put CREDENTIAL_ENCRYPTION_KEY --env production
npx wrangler secret put INTERNAL_CONTAINER_TOKEN --env production
npx wrangler secret put MCP_BEARER_TOKEN --env production
npx wrangler secret put CF_ACCESS_AUD --env production
```

Push to `main` or run the **deploy** workflow manually. The pipeline:

1. Builds `web/dist`
2. Builds and pushes the container image (`wrangler containers build -p`)
3. Deploys the Worker with the registry image (`wrangler deploy --env production`)

Container receives runtime env (`COLD_CLI_DATABASE_URL` / `DATABASE_URL`, encryption key, Google client, internal token, `PUBLIC_BASE_URL`) forwarded from Worker secrets on container start.

Cron: `*/2 * * * *` (UTC). Cron does not imply a send.

## Manual deploy (local machine with Docker)

```bash
cd web && npm install && npm run build
cd ../worker && npm install --legacy-peer-deps

# Substitute placeholders in wrangler.jsonc production env, then:
export TAG=$(git rev-parse --short HEAD)
sed -i "s|__OPENOUTREACH_HOSTNAME__|outreach.example.com|g" wrangler.jsonc
sed -i "s|__CLOUDFLARE_ACCOUNT_ID__|$CLOUDFLARE_ACCOUNT_ID|g" wrangler.jsonc
sed -i "s|__IMAGE_TAG__|$TAG|g" wrangler.jsonc

npx wrangler containers build -p -t "outreachd:$TAG" ..
npx wrangler deploy --env production
```

Docker must be running for `containers build`.

## Cloudflare Access

Create a Zero Trust application for your hostname. Protect `/`, `/api/*`, `/mcp/*`. Add bypass policies for:

- `/t/*` (open/click tracking pixels — fetched by mail clients without a session)
- `/api/v1/accounts/google/oauth/callback` (Google OAuth redirect)

Set `CF_ACCESS_AUD` to your Access application **AUD tag**. The Worker verifies the `Cf-Access-Jwt-Assertion` JWT signature (RS256) and audience before serving protected routes.

MCP clients can use `Authorization: Bearer $MCP_BEARER_TOKEN` instead of Access when calling `/mcp`.

## Post-deploy smoke test

1. `GET https://<host>/internal/health` (via internal token or temporarily without Access) — `database: true`, `mock_gmail: false`
2. Open dashboard through Access — Overview loads
3. Sending Accounts → Connect Google — OAuth round trip succeeds
4. Create campaign → preview → activate with `confirm: true`
5. Wait for cron tick — email sends, `last_successful_tick` advances
6. Reply from recipient — appears in Inbox, sequence suppresses

## Local compose

```bash
docker compose up --build
```

Postgres on `localhost:5433`, outreachd on `:8080`, Vite on `:5173`.

## Operations runbook

| Task | Action |
|------|--------|
| Rotate encryption key | Not supported in-place — decrypt/re-encrypt `google_credentials` or reconnect all Google accounts after key change |
| Reconnect paused account | Sending Accounts → Connect Google for the same mailbox |
| Failed tick | Check `/internal/health` for `last_successful_tick`; inspect container logs; verify Postgres is direct (not pooled) |
| Redeploy | Push to `main` or re-run deploy workflow |
