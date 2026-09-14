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
| outreach_setup | First-run counts + next actions |
| outreach_create_campaign | **draft only — does not send**; CSV lands on the shortlist |
| outreach_update_campaign | Draft/paused only; never activates |
| outreach_clone_campaign | Always stays **draft** |
| outreach_preview_campaign / outreach_review_campaign | Rendered emails + readiness |
| outreach_preview_sequence | Renders the YAML you pass |
| outreach_activate_campaign | **consequential — requires `confirm: true`** |
| outreach_pause_campaign / resume | |
| outreach_get_playbook / put_playbook | Workspace facts for fit + drafts |
| outreach_list_shortlist / add_shortlist / patch_shortlist | Review before enrollment |
| outreach_enroll_shortlist | Schedules approved people; still does not send |
| outreach_add_leads / remove_lead / validate_leads | Enroll schedules; prefer shortlist first |
| outreach_get_campaign / list_campaigns / get_campaign_stats | |
| outreach_list_replies / get_thread / reply_to_thread | reply requires `confirm: true` + `confirm_to` |
| outreach_handoff_thread / retry_handoff / patch_thread | Owner + visible delivery; retry does not repeat outreach |
| outreach_draft_sequence | Uses playbook audience/offer when you omit ICP; never activates |
| outreach_search_leads / blacklist_lead | `?q=` searches email/name/company |
| outreach_list_suppressions / add_suppression | Global block list; honored on import |
| outreach_verify_leads | Syntax + MX + disposable; no API key |
| outreach_list_capabilities | Operator feature flags (no secrets) |
| outreach_list_integrations | Masked workspace API keys |
| outreach_put_integration | Create/rotate credential; secret never echoed |
| outreach_test_integration | Live/local credential probe |
| outreach_delete_integration | Delete credential by id |
| outreach_apollo_search | Preview Apollo people → CSV; does not activate |
| outreach_search_leads | Workspace search, or `provider=apollo` connector preview |
| outreach_enrich_lead | Email enrich preview (local + connector) |
| outreach_sheets_import | Public Sheets/CSV URL → shortlist; does not activate |
| outreach_import_leads | CSV onto the shortlist (not enroll) |
| outreach_preflight_campaign | Non-mutating readiness checks |
| outreach_suggest_reply | Reports `used_playbook`; send still needs confirm |

Parity rule: Settings / Accounts actions that agents need have an MCP twin. Tokens are never returned.

## Safety — no silent sends

Policy: **create ≠ send**. Activation is never inferred from earlier tool calls.

Normal agent loop:

```
playbook → shortlist → draft campaign → review rendered emails → explain blockers → human activate (confirm: true)
```

Recommended session:

1. `outreach_get_playbook` — audience, offer, schedule for this workspace
2. `outreach_create_campaign` — draft; `leads_csv` goes to the shortlist
3. `outreach_add_shortlist` / `outreach_list_shortlist` / `outreach_patch_shortlist` — approve with a written fit reason
4. `outreach_enroll_shortlist` — schedules only after a mailbox is assigned
5. `outreach_review_campaign` or `outreach_preview_sequence` — return the actual emails, not YAML-only
6. **Stop** until a human approves; only then `outreach_activate_campaign` with `confirm: true`
7. Replies: `outreach_get_thread` + `outreach_reply_to_thread` (confirm). Interested: `outreach_handoff_thread` (confirm). Retry failed delivery with `outreach_retry_handoff` — do not send the sequence again.

Do not pass the campaign name as ICP. `outreach_draft_sequence` fills audience/offer from the playbook when omitted. Suggested replies report `used_playbook` honestly.

Responses include `status`, counts, `warnings`, and `next_actions` where useful. Treat `warnings` as blocking signals for activate.

## REST parity

HTTP API under `/api/v1/*` uses the same engine. Envelope: `{ data, error, warnings }` (+ `next_actions` where useful). See [ARCHITECTURE.md](ARCHITECTURE.md) and [INTEGRATIONS.md](INTEGRATIONS.md).

Mintlify publish target ([#21](https://github.com/sdntsng/openoutreach/issues/21)): this file is the canonical Agents (MCP) section until a Mintlify docs site is wired.
