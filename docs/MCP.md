# MCP

Endpoint: `POST /mcp` (Worker).

Auth: `Authorization: Bearer $MCP_BEARER_TOKEN` when configured (or Cloudflare Access JWT when `CF_ACCESS_AUD` is set).

## Tools

| Tool | Notes |
|------|--------|
| outreach_list_accounts | |
| outreach_get_account_status | |
| outreach_create_campaign | **draft only — does not send** |
| outreach_preview_campaign | |
| outreach_activate_campaign | **consequential — explicit human approval** |
| outreach_pause_campaign / resume | |
| outreach_add_leads / remove_lead / validate_leads | |
| outreach_get_campaign / list_campaigns / get_campaign_stats | |
| outreach_list_replies | |
| outreach_get_thread / reply_to_thread | Requires `campaign_id` + `lead_id` |
| outreach_search_leads / blacklist_lead | Search uses `q`; blacklist by `lead_id` or `email` |

## Safety

Normal agent workflow:

```
create → add leads → preview → human reviews → activate
```

Activation is never inferred from earlier tool calls. Responses include `status`, counts, `warnings`, and `next_actions`. Tokens are never returned.
