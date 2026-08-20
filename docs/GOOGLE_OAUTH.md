# Google OAuth

## Scopes (least privilege)

- `openid`
- `email`
- `https://www.googleapis.com/auth/gmail.send`
- `https://www.googleapis.com/auth/gmail.readonly`

## Dashboard flow

Settings / Sending Accounts → **Connect Google** → Google consent → callback → account `active`.

API:

- `POST /api/v1/accounts/google/oauth/start` → `{ authorize_url, state }`
- `GET /api/v1/accounts/google/oauth/callback?code&state`

State is stored server-side and single-use.

## Token storage

- Refresh (+ access when present) encrypted with AES-256-GCM using `CREDENTIAL_ENCRYPTION_KEY`
- Table: `google_credentials`
- Never logged, never returned from API/MCP, never in localStorage

## Failures

`invalid_grant` / 401 → pause account, set `oauth_health=reconnect_required`. Do not reassign leads to another mailbox (breaks threads).

## Local mock

`OPENOUTREACH_MOCK_GMAIL=1` — `POST .../oauth/start?email=you@example.com` creates a mock GWS account without Google.
