# OpenOutreach

[![CI](https://github.com/sdntsng/openoutreach/actions/workflows/ci.yml/badge.svg)](https://github.com/sdntsng/openoutreach/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-3d9b84.svg)](LICENSE)

**Your outbound workspace. Your mailbox. Your agents. You decide what sends.**

Self-hosted, agent-first cold email for a founder or small team working a modest, researched list. Built on [cold-cli](https://github.com/andersmyrmel/cold-cli) (MIT). Humans use the dashboard. Agents use MCP/API. Both drive the same engine:

**contacts → sequence → schedule → send → thread → reply → suppress → analytics**

Create never sends. Activate is explicit. Mail goes out from a mailbox you own.

## What a new operator does

1. **Deploy your instance** (local docker, or Cloudflare — see below). This is *your* workspace, not a shared pool.
2. Open the **dashboard** and fill **Project** (who you are, who you want, the offer).
3. **New campaign** — write the emails in the visual editor. Connecting Gmail is not required to draft.
4. Import ~20 people onto the **shortlist**. See company, role, why they fit, email check, and previous outreach. Approve or exclude.
5. **Review** the actual messages beside the sequence, with recipient, window, and readiness problems.
6. **Activate** only when the checklist is green (`confirm: true` on API/MCP).
7. Answer replies **in-thread**. Mark interest and **route** the conversation; you should see whether the handoff landed. Retry failed delivery without sending the outreach again.

An agent can retrieve the playbook, prepare the same shortlist and draft, return a rendered preview, and explain blockers. Activation stays a human approval. Both see the same state.

## Local path (your mailbox, working example)

```bash
cp .env.example .env
docker compose up --build
```

- Dashboard: `http://localhost:5173`
- API / tick: `http://localhost:8080`

With `OPENOUTREACH_MOCK_GMAIL=1` you can walk the motion without OAuth. For a real mailbox, connect Google on **Integrations** (`openid email gmail.send gmail.readonly`). If OAuth fails, the account shows **Reconnect required** or **Saved; connection not verified** — those are different from “a row exists.” Send-only providers (e.g. Resend) can send; they cannot ingest replies here.

Recovery: pause the campaign, reconnect the mailbox, retry a failed handoff from Inbox. Tick never sends while the tick lock is held.

### API example (same engine; still does not send until activate)

```bash
curl -s localhost:8080/internal/health | jq
curl -s -X POST localhost:8080/api/v1/campaigns \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo","sequence_yaml":"name: demo\ndefaults:\n  from_name: You\nsteps:\n  - step: 1\n    delay: 0\n    subject: Hi {{first_name}}\n    body: Hello {{first_name}}\n","leads_csv":"email,first_name,company\nprospect@acme.com,Ada,Acme\n"}' | jq
# People are on the shortlist. Approve, enroll, GET /campaigns/{id}/review, then:
curl -s -X POST localhost:8080/api/v1/campaigns/demo/activate \
  -H 'Content-Type: application/json' \
  -d '{"confirm":true}' | jq
```

## Deploy to Cloudflare

**New account:** follow [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) so the Worker, D1/Postgres, encryption key, and OAuth callback are *yours*. Portable exports (leads CSV, campaign leads) and additive migrations are how you keep ownership across upgrades.

**Existing Worker `openoutreach`:** connect this repo in the dashboard. Do **not** use the Deploy button — it can fork and create a second Worker.

1. [Workers & Pages](https://dash.cloudflare.com/?to=/:account/workers-and-pages) → **openoutreach** → **Settings → Builds → Connect**
2. GitHub repo **`sdntsng/openoutreach`**, production branch `main`, root directory `worker`
3. Build `npm run build` · deploy `npm run deploy:worker`

[![Deploy to Cloudflare](https://deploy.workers.cloudflare.com/button)](https://deploy.workers.cloudflare.com/?url=https://github.com/sdntsng/openoutreach&dir=worker)

```bash
cp .env.deploy.example .env.deploy
./scripts/deploy-cf.sh
```

## Docs

| Doc | Purpose |
|-----|---------|
| [AGENTS.md](AGENTS.md) | Agent / contributor context graph |
| [docs/PARITY.md](docs/PARITY.md) | Closed/open-source comparison + in-app gaps we closed |
| [docs/INTEGRATIONS.md](docs/INTEGRATIONS.md) | Lead sources, send providers, Settings/MCP roadmap |
| [docs/UPSTREAM.md](docs/UPSTREAM.md) | cold-cli fork baseline |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Hosted topology |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Cloudflare + Postgres rules |
| [docs/GOOGLE_OAUTH.md](docs/GOOGLE_OAUTH.md) | OAuth scopes & vault |
| [docs/MCP.md](docs/MCP.md) | Agent tools |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat notes |
| [docs/RELEASING.md](docs/RELEASING.md) | Semver tags + CI release |
| [CONTRIBUTING.md](CONTRIBUTING.md) | PR / test norms |

## License

MIT — see [LICENSE](LICENSE) and [NOTICE](NOTICE) (includes upstream **cold-cli** attribution).
