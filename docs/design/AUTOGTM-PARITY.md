# AutoGTM (explee) gap analysis

Reference: closed-source AutoGTM screenshots (dashboard, new campaign, campaign workspace, suppress list, integrations, targeting, email templates, project docs, mailbox, API keys). Used as **IA / UX parity**, not as a license to copy their SaaS model.

OpenOutreach stays: **create ≠ send**, eager `scheduled_sends`, `GWSClient` only, tick lock caller-owned, workspace from config/header, approx. opens never block send. No LinkedIn scrape, no Instantly/Smartlead send backends, no billing.

## What we will not build

| AutoGTM surface | Why not |
|-----------------|---------|
| Payment method / daily $ budget / cost-per-lead / top-up | SaaS billing. We are self-hosted. |
| Autopilot toggle (“41% more replies”) and auto-pause with AI reasons | Would send or pause without explicit activate/confirm. Optional later, never inside `engine.Tick`. |
| Native HubSpot / Salesforce / Pipedrive | Outbound webhook + MCP already route hot leads. Native CRMs are a later connector. |
| LinkedIn match on suppress / LinkedIn steps | ToS-hostile. Email + domain only. |
| “Steer the campaign…” chat / auto-fill ICP from a name | No new LLM compose engine. `draft-sequence` is ReplaceAll YAML, human activate. |
| Team / Billing / Refer / theme switcher | Single `workspace_id` per deploy. |
| Lead-language auto-translation | Out of scope. |

## What we already have (buried or incomplete)

| Capability | Today | Gap |
|------------|--------|-----|
| Overview metrics | Sent, replies, rate, positive, bounces, approx. opens | No campaign table on Overview |
| Campaigns table | Name, status, leads, sent, replies, opens | Missing reply %, interested, status chips |
| New campaign | 5-step wizard, YAML | No Compose vs Import split; `draft_only: false` on create is confusing |
| Campaign detail | Overview / sequence / preview / stats | Not Campaign vs Leads; sequence is a raw dump |
| Inbox | Flat inbound list | No Needs reply / Got reply / Sent; no campaign filter; no Hot |
| Suppressions | API + buried on Leads | No dedicated page; no People vs Companies |
| Integrations | Connector hub + vault | No featured “route replies” card or agent API snippet |
| Sequences | Per-campaign YAML | No first-class templates / schedule / targeting / project pages |
| Settings / MCP | Workspace + bearer indicator | Keys stay operator-set; no sk_ browser secrets |

## Implementation order (this work)

1. **Shell IA** — Overview · Mailbox (Inbox badge, Sent) · Setup (Project, Templates, Schedule, Targeting, Integrations, Suppress list, Leads, Sending Accounts) · Campaigns list + New · Settings in the user footer.
2. **Suppress list** — Dedicated `/suppressions`. People = emails + CSV. Companies = domain paste + CSV. Same `suppressions` API.
3. **Inbox** — `?box=needs|replies|sent`. Campaign filter. Classification / Hot. Empty states.
4. **Campaigns** — Reply rate + interested. Status chips. New campaign: Compose (name → draft, optional `draft-sequence`) vs Import leads. Detail: Campaign / Leads + pause banner.
5. **Workspace playbook** — `GET/PUT /api/v1/workspace/playbook` in `hosted_kv`. Powers Project, Targeting, Schedule, Templates (default YAML + live ReplaceAll preview). Not SuperSearch. Not AI autopilot.
6. **Overview + Integrations polish** — Campaign table on Overview. Featured outbound webhook + copy-paste API snippet for agents.

## UX principles (dashboard)

Light surfaces, mint/teal accent, Inter, ~8–10px radius, thin hierarchy. Instructional copy next to the control. Status chips, not raw strings. Dashed CSV drop. Primary actions enabled only when the form is valid. Never imply that create sends mail.
