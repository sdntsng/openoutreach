# Changelog

All notable changes to OpenOutreach are documented here.

## Unreleased

### Added

- Campaign review: visual email steps beside a recipient preview; YAML is the advanced view over the same format. Activate shows recipients, exclusions, account, window, and readiness fixes. Create no longer requires a mailbox.
- Shortlist review (CSV / Apollo / Sheets / agent / Clay ingest): company, role, fit reason, source/date, syntax email check, previous outreach. Approve or exclude before enrollment.
- Actionable Overview for this workspace (leads awaiting review, drafts, replies needing attention, failed handoffs). Connector catalog stays on Integrations. No LinkedIn “Soon”; warmup is not on Overview.
- Conversation owner, needs-action, and persist-per-attempt outbound handoff. HTTP errors no longer skip later events. Retry does not repeat outreach.
- MCP loop: playbook → shortlist → draft → review → human activate. Preview returns rendered emails.

### Changed

- Everyday copy: prepare/review before sending; next-send status instead of scheduler jargon; “Saved; connection not verified” when that is what we know.
- README leads with the operator experience and cold-cli attribution, not curl-first setup.

## v0.1.0

Initial public release (tag cut from `main`).
