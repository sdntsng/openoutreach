# OpenOutreach

[![CI](https://github.com/sdntsng/openoutreach/actions/workflows/ci.yml/badge.svg)](https://github.com/sdntsng/openoutreach/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-3d9b84.svg)](LICENSE)

Self-hosted, **agent-first** cold email outreach built on [cold-cli](https://github.com/andersmyrmel/cold-cli).

Humans use the dashboard. Agents use MCP/API. Both operate the same deterministic engine:

**contacts → sequence → schedule → send → thread → reply → suppress → analytics**

## 10-minute quickstart (local)

```bash
cp .env.example .env
docker compose up -d postgres
export $(grep -v '^#' .env | xargs)
export OPENOUTREACH_MOCK_GMAIL=1
go run ./cmd/outreachd
```

In another terminal:

```bash
curl -s localhost:8080/internal/health | jq
# Connect mock Gmail account
curl -s -X POST 'localhost:8080/api/v1/accounts/google/oauth/start?email=you@example.com' | jq
```

Create a draft campaign (does **not** send):

```bash
curl -s -X POST localhost:8080/api/v1/campaigns \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"demo",
    "accounts":["you@example.com"],
    "sequence_yaml":"name: demo\ndefaults:\n  from_name: You\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi {{first_name}}\n    body: Hello {{first_name}}\n",
    "leads_csv":"email,first_name\nprospect@acme.com,Ada\n"
  }' | jq
```

Preview, then activate only when ready (`confirm` required):

```bash
curl -s localhost:8080/api/v1/campaigns/demo/preview?render=1 | jq
curl -s -X POST localhost:8080/api/v1/campaigns/demo/activate \
  -H 'Content-Type: application/json' \
  -d '{"confirm":true}' | jq
curl -s -X POST localhost:8080/internal/tick -H 'X-Internal-Token: '"$INTERNAL_CONTAINER_TOKEN" | jq
```

Dashboard: `cd web && npm install && npm run dev` (proxies `/api` or point Vite at `:8080`).

## Docs

| Doc | Purpose |
|-----|---------|
| [AGENTS.md](AGENTS.md) | Agent / contributor context graph |
| [docs/UPSTREAM.md](docs/UPSTREAM.md) | cold-cli fork baseline |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Hosted topology |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Cloudflare + Postgres rules |
| [docs/GOOGLE_OAUTH.md](docs/GOOGLE_OAUTH.md) | OAuth scopes & vault |
| [docs/MCP.md](docs/MCP.md) | Agent tools |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat notes |
| [docs/RELEASING.md](docs/RELEASING.md) | Semver tags + CI release |
| [CONTRIBUTING.md](CONTRIBUTING.md) | PR / test norms |

## Suggested GitHub metadata

When publishing, set repository topics:

`cold-email` · `outreach` · `gmail-api` · `mcp` · `cloudflare-workers` · `golang` · `self-hosted` · `sqlite` · `postgres`

About blurb:

> Self-hosted agent-first cold email: cold-cli engine + Gmail OAuth + dashboard + MCP.

## License

MIT — see [LICENSE](LICENSE) and [NOTICE](NOTICE) (includes upstream cold-cli attribution).
