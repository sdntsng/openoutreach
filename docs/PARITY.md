# Product parity

Researched comparison of OpenOutreach against the main **closed-source** cold-email / sales-engagement products and the main **open-source** outreach or email-ops tools. Used to decide what we implement here: gaps that stay MIT/self-hosted, can be turned on from the **web app**, and do not need extra developer or environment wiring.

**Not in scope (locked):** Instantly/Smartlead as send backends; in-product LinkedIn/Sales Nav scraping; warmup traffic inside `engine.Tick`; multi-tenant SaaS billing.

Sources: public product pages and 2026 comparison writeups for Instantly, Smartlead, Lemlist, Apollo, Woodpecker, Reply.io, Mailshake, Outreach, Salesloft; GitHub READMEs for Warmbly, Quickly, ShoutReach, Linki, Listmonk, Mautic, Keila, Postal, and upstream [cold-cli](https://github.com/andersmyrmel/cold-cli). Prices and marketing claims change — treat competitor cells as **feature presence**, not a buying guide.

Legend: **Y** = we have it in dashboard or API/MCP · **P** = partial · **N** = we do not, or we refuse on purpose · **—** = not that product's job.

---

## 1. Closed-source (what operators pay for)

| Capability | Instantly | Smartlead | Lemlist | Apollo | Woodpecker | Reply.io | Mailshake | Outreach / Salesloft | **OpenOutreach** |
|------------|-----------|-----------|---------|--------|------------|----------|-----------|----------------------|------------------|
| Multi-mailbox send (own Gmail/M365/SMTP) | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Sequences + delays + send windows | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| A/B variants | Y | Y | Y | Y | Y | Y | Y | Y | **Y** (YAML) |
| Unified inbox + stop-on-reply | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Bounce / unsubscribe suppress | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Daily caps + account pause | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Open/click tracking | Y | Y | Y | Y | Y | Y | Y | Y | **P** (opens labeled Approx.; clicks stored, no UI) |
| Built-in lead database | Y (SuperSearch) | add-on | credits | **core** | N | credits | N | add-on | **N** — bring Apollo/Clay/CSV (OSS-friendly) |
| Warmup network | included | included / add-on | Lemwarm | partner | add-on | add-on | add-on | partner | **P** — vault badge only; never Tick |
| Inbox placement tests | add-on | add-on | tips | N | N | N | N | add-on | **N** — needs third-party seed list |
| DNS / SPF / DKIM wizard | Y | SmartSenders | Y | N | tips | tips | tips | IT-owned | **Y** — read-only DNS from the app (no extra token) |
| Email verify / MX | bundled | bundled | bundled | reveal | basic | bundled | basic | data vendors | **Y** — syntax + MX + disposable, no API key |
| Global suppression list | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Campaign clone | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| CSV / Sheets import | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| CSV export | Y | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Outbound webhooks (reply → Slack/CRM) | Y | Y | Y | Y | Y | Y | Y | Y | **Y** — URL in Settings vault |
| Visual sequence builder | Y | Y | Y | Y | Y | Y | Y | Y | **P** — YAML + draft-from-brief |
| LinkedIn steps | N / limited | N | **Y** | Y | N | Y | N | Y | **N** — webhook ingest only (ToS) |
| Agency workspaces / white-label | folders | **Y** | seats | seats | seats | seats | seats | enterprise | **P** — one `workspace_id` per deploy |
| API + agent/MCP | REST | deep REST | REST | REST | REST | REST | REST | REST | **Y** — REST + MCP, create ≠ send |
| Self-host / your data | N | N | N | N | N | N | N | N | **Y** |

**Where they are ahead on purpose:** paid lead graphs, shared warmup pools, placement seed networks, LinkedIn automation, agency white-label. Those are either closed data products, ToS-hostile, or a different company shape. We compete on **own-mailbox send + sequences + inbox + agents**, not on renting Instantly's network.

---

## 2. Open-source

| Capability | cold-cli (upstream) | Warmbly | Quickly | ShoutReach | Linki | Listmonk / Keila | Mautic | Postal | **OpenOutreach** |
|------------|---------------------|---------|---------|------------|-------|------------------|--------|--------|------------------|
| Cold sequences (not newsletters) | Y (CLI) | Y | Y | Y | Y | N | P | N | **Y** + dashboard |
| Own mailboxes (OAuth + SMTP) | GWS CLI / SMTP | Y | Y | SMTP/IMAP | SMTP | SMTP | SMTP | MTA | **Y** Gmail, M365, SMTP, Resend, CF Email |
| Hosted dashboard | N | Y | Y | Y | Y | Y | Y | Y | **Y** |
| Agent / MCP | N | AI-native | API | N | managed add-on | N | N | N | **Y** |
| Create ≠ send | CLI discipline | varies | varies | varies | varies | campaigns = send | automations | N | **Y** — hard invariant |
| One-click / compose deploy | N | docker | Railway/VPS | VPS | app store | docker | docker | docker | **Y** — Deploy to Cloudflare + `deploy-cf.sh` + D1 |
| Warmup in-process | N | **Y** | P | N | N | N | N | N | **N** in Tick (badge only) |
| Maps / LinkedIn scrape | N | N | N | Maps | LinkedIn | N | N | N | **N** |
| Transactional / list mail | N | N | N | N | N | **Y** | **Y** | **Y** | secondary (Resend/CF) |

Listmonk, Keila, Mautic, and Postal are **list/transactional** tools. We do not try to replace them. Warmbly/Quickly are the closest Instantly-class OSS peers; we stay agent-first and refuse scrape/warmup-in-engine.

---

## 3. Gaps we closed in-app (this work)

Configured from the dashboard or vault. No new required env vars. `FEATURE_*` for Resend / CF Email / Hunter / warmup now **default on** so the forms appear after deploy; set `=0` only to hide a provider.

| Gap | Instantly-class need | What we shipped | Setup |
|-----|----------------------|-----------------|-------|
| First-run | “Connect mailbox → import → draft” | Overview setup checklist | Open `/` |
| Suppression list | Global block emails/domains | Leads → suppressions; honored on import | CSV or one-line |
| Email verify | Don't burn domains on junk | Validate + verify (syntax, MX, disposable) | No key |
| DNS health | SPF / MX / DMARC before send | Accounts → Check DNS | Public DNS only |
| Campaign clone | Duplicate sequence + leads as **draft** | Campaign detail → Clone | Confirm stays draft |
| Campaign PATCH | Edit draft/paused YAML & windows | API + MCP (already referenced) | Web/API |
| Export | Take data out | Leads CSV, campaign leads CSV | Button |
| Outbound webhook | Reply/bounce/sent → Slack/Make/HubSpot | Settings vault `outbound` | Paste URL |
| Lead search | Find by email/name/company | Leads search box hits those fields | — |
| Provider flags | Hide CF Email until wrangler secret | Defaults on; vault is the real switch | Web forms |
| Preflight / windows / tracking | Hidden API | Create wizard + detail | Checkboxes |

Still **not** built (and why): inbox placement SaaS, paid SuperSearch clone, LinkedIn steps, warmup sending, white-label agency SaaS, visual drag-and-drop sequencer (YAML + draft-sequence is enough for OSS).

---

## 4. How to turn things on (no extra deploy)

1. Deploy once (`./scripts/deploy-cf.sh` or the Deploy to Cloudflare button). Encryption key is already required.
2. Open the dashboard → Overview checklist.
3. **Sending Accounts:** Connect Google / Microsoft, or paste SMTP / Resend / Cloudflare Email from the form. No `FEATURE_CF_EMAIL=1` secret required unless you want to *disable* it.
4. **Leads:** CSV, Sheets URL, Apollo (paste key in Settings), or Clay webhook.
5. **Settings:** optional Apollo/Clay/Sheets/outbound URL. Secrets stay in the vault.
6. Create a **draft** campaign, preview, activate with confirm.

Operator kill-switches remain `FEATURE_*=0` for air-gapped or policy-locked instances.
