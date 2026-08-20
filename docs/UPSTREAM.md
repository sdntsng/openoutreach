# Upstream cold-cli audit

Fork base: [andersmyrmel/cold-cli](https://github.com/andersmyrmel/cold-cli) @ `22edd27c01607c503fb07eb26f30e480bb744f38`

## Test baseline (Phase A)

```
go test ./...
ok  cmd/cold-cli
ok  internal
ok  pkg/engine
```

Re-run after hosted changes: `go test ./internal/hosted/ ./pkg/engine/ ./internal/ ./cmd/...`

## What OpenOutreach consumes

| Primitive | Location | Hosted usage |
|-----------|----------|--------------|
| Eager scheduler | `internal/scheduler.go` | Unchanged |
| Tick engine | `internal/tick.go` via `engine.Tick` | Cron → `POST /internal/tick` |
| Advisory lock | `Store.AcquireTickLock` (caller-owned) | outreachd acquires before Tick |
| GWSClient | `internal/gws.go` | `GoogleAPIProvider` + `MockGmail` + CLI `GWSCLI` |
| SecretResolver | `internal/secrets.go` | Hosted vault for Google; SMTP `secret:` refs |
| Threading | `internal/send.go` | Unchanged |
| Reply/bounce/unsub | `internal/reply.go` | Unchanged |
| Preview / activate | `internal/campaign.go` | REST + MCP |
| Workspaces | `workspace_id` on accounts/campaigns | Header `X-Workspace-ID` / env |

## Hosted deltas (minimal)

- `TickConfig.MaxSendsPerTick` + `TrackingPixelForSend`
- `EmailParams.TrackingPixelURL`
- `pkg/engine` re-exports for control plane
- `internal/hosted` OAuth vault, API, Google API provider
- Additive tables via `BootstrapHostedSchema`

## Do not rewrite

Eager `scheduled_sends`, rebalance, Postgres advisory locks, reply cancellation semantics, CLI entrypoint.
