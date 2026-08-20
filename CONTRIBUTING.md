# Contributing

Thanks for helping with OpenOutreach.

## Before you start

Read [AGENTS.md](AGENTS.md) — product invariants and layout. Do not rewrite the eager scheduler, tick locking, or `GWSClient` contract.

## Dev setup

```bash
cp .env.example .env
docker compose up -d postgres
export OPENOUTREACH_MOCK_GMAIL=1
export COLD_CLI_DATABASE_URL=postgresql://openoutreach:openoutreach@localhost:5433/openoutreach?sslmode=disable
go run ./cmd/outreachd
```

Dashboard: `cd web && npm install && npm run dev`  
Worker (optional): `cd worker && npm install --legacy-peer-deps`

## Checks

```bash
go test ./internal/hosted/ ./pkg/engine/ ./internal/
cd web && npm run build
cd worker && npx tsc --noEmit
```

## PRs

- One logical change per PR
- Keep hosted diffs to `internal/hosted`, `pkg/engine` re-exports, and minimal upstream hooks
- Never commit secrets
- For UI: follow the light shadcn-like teal aesthetic in `AGENTS.md`

## License

MIT — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
