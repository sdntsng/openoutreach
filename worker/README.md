# Worker (Cloudflare edge)

Proxies API, cron tick, tracking, MCP, and static dashboard assets to the `outreachd` Container.

Dashboard sign-in is Better Auth on this Worker (`/sign-in`, `/api/auth/*`). Tracking `/t/*` and Gmail OAuth callbacks stay public.

## Deploy

See [docs/DEPLOYMENT.md](../docs/DEPLOYMENT.md).

Quick path from repo root:

```bash
cp .env.deploy.example .env.deploy
# set DATABASE_URL
./scripts/deploy-cf.sh
```

Local dev:

```bash
cp .dev.vars.example .dev.vars
npm install --legacy-peer-deps
npm run typecheck
npm run dev
```
