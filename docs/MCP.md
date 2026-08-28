# MCP

Endpoint: `POST /mcp` on the Worker URL (same origin as the dashboard host).

Auth: `Authorization: Bearer $MCP_BEARER_TOKEN` when configured. Bearer bypasses Cloudflare Access on `/mcp` only.

## Cursor MCP config

Hosted OpenOutreach MCP is **remote HTTP** (`POST /mcp` on the Worker). Cursor can attach either as a native remote server or via a stdio bridge that still talks to that same URL.

### Remote (preferred)

```json
{
  "mcpServers": {
    "openoutreach": {
      "url": "https://YOUR_WORKER.workers.dev/mcp",
      "headers": {
        "Authorization": "Bearer ${env:MCP_BEARER_TOKEN}"
      }
    }
  }
}
```

Replace `YOUR_WORKER.workers.dev` with your Worker hostname. Never commit the bearer token.

### stdio (local process wrapping the hosted URL)

Use stdio when the client only launches a local command. Point the bridge at the **same** Worker `/mcp` URL — this is not a separate local engine.

```json
{
  "mcpServers": {
    "openoutreach": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://YOUR_WORKER.workers.dev/mcp", "--header", "Authorization: Bearer ${MCP_BEARER_TOKEN}"]
    }
  }
}
```

Do not configure a stdio `cold-cli` MCP as a send path. Create ≠ send still applies: `outreach_activate_campaign` and `outreach_reply_to_thread` require `confirm: true`.

## Tools

| Tool | Notes |
|------|--------|
| outreach_list_accounts | |
| outreach_add_cf_email_account | Vaulted API token; `FEATURE_CF_EMAIL` |
| outreach_pause_account / resume | Tick will not send while paused |
| outreach_get_account_status | |
| outreach_create_campaign | **draft only — does not send** |
| outreach_update_campaign | |
| outreach_preview_campaign | |
| outreach_activate_campaign | **consequential — requires `confirm: true`** |
| outreach_pause_campaign / resume | |
| outreach_add_leads / remove_lead / validate_leads | |
| outreach_get_campaign / list_campaigns / get_campaign_stats | |
| outreach_list_replies / get_thread / reply_to_thread | reply requires `confirm: true` + `confirm_to` |
| outreach_search_leads / blacklist_lead | |
| outreach_list_capabilities | Operator feature flags (no secrets) |
| outreach_list_integrations | Masked workspace API keys |
| outreach_put_integration | Create/rotate credential; secret never echoed |
| outreach_test_integration | Live/local credential probe |
| outreach_delete_integration | Delete credential by id |
| outreach_apollo_search | Preview Apollo people → CSV; does not activate |
| outreach_search_leads | Workspace search, or `provider=apollo` connector preview |
| outreach_enrich_lead | Email enrich preview (local + connector) |
| outreach_sheets_import | Public Sheets/CSV URL → preview or append; does not activate |
| outreach_import_leads | Append CSV; `dry_run` preview; active campaign needs `confirm` |
| outreach_draft_sequence | ICP/offer → YAML draft only |
| outreach_preflight_campaign | Non-mutating readiness checks |
| outreach_suggest_reply | Classification-based suggestion; send still needs confirm |

Parity rule: Settings / Accounts actions that agents need have an MCP twin. Tokens are never returned.

## Safety — no silent sends

Policy: **create ≠ send**. Activation is never inferred from earlier tool calls.

Normal agent loop:

```
create draft → add leads → preview → human reviews → activate (confirm: true)
```

Recommended session:

1. `outreach_list_capabilities` — see which providers the operator enabled
2. `outreach_draft_sequence` — get YAML; create campaign via `outreach_create_campaign` (draft)
3. `outreach_apollo_search` or `outreach_sheets_import` / `outreach_import_leads` — preview then import
4. `outreach_preview_campaign` + `outreach_preflight_campaign`
5. **Stop** until a human approves; only then `outreach_activate_campaign` with `confirm: true`

Responses include `status`, counts, `warnings`, and `next_actions` where useful. Treat `warnings` as blocking signals for activate.

## REST parity

HTTP API under `/api/v1/*` uses the same engine. Envelope: `{ data, error, warnings }` (+ `next_actions` where useful). See [ARCHITECTURE.md](ARCHITECTURE.md) and [INTEGRATIONS.md](INTEGRATIONS.md).

Mintlify publish target ([#21](https://github.com/sdntsng/openoutreach/issues/21)): this file is the canonical Agents (MCP) section until a Mintlify docs site is wired.
