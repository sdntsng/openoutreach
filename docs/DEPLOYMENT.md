# Deployment

## Critical: Postgres for tick

**Use a direct Postgres connection string for outreachd / Container.**

Do **not** use:

- PgBouncer **transaction** pooling
- Cloudflare Hyperdrive for the tick / advisory-lock connection

`cold-cli` uses `pg_try_advisory_lock` on a dedicated session connection. Pooled transaction modes break lock semantics and can duplicate sends.

Worker tracking inserts may use Hyperdrive later; V1 routes tracking through the Container.

## Cloudflare

```bash
cd worker
npm install --legacy-peer-deps
# set secrets
echo -n "$DATABASE_URL" | npx wrangler secret put DATABASE_URL
npx wrangler secret put GOOGLE_CLIENT_ID
npx wrangler secret put GOOGLE_CLIENT_SECRET
npx wrangler secret put CREDENTIAL_ENCRYPTION_KEY
npx wrangler secret put INTERNAL_CONTAINER_TOKEN
npx wrangler secret put MCP_BEARER_TOKEN

cd ../web && npm install && npm run build
cd ../worker && npx wrangler deploy
```

Container receives runtime env (`COLD_CLI_DATABASE_URL` / `DATABASE_URL`, encryption key, Google client, internal token, `PUBLIC_BASE_URL`) forwarded from Worker secrets on container start.

Cron: `*/2 * * * *` (UTC). Cron does not imply a send.

## Cloudflare Access

Protect `/`, `/api/*`, `/mcp/*` at the Access application. Bypass `/t/*` and OAuth callback paths.

When `CF_ACCESS_AUD` is set on the Worker, requests without `Cf-Access-Jwt-Assertion` are rejected (except public tracking/OAuth). MCP clients can use `Authorization: Bearer $MCP_BEARER_TOKEN` instead of Access service tokens.

## Local compose

```bash
docker compose up --build
```

Postgres on `localhost:5433`, outreachd on `:8080`, Vite on `:5173`.
