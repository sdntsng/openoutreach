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

## Production setup

1. Create a Google Cloud OAuth 2.0 **Web application** client.
2. Add authorized redirect URIs:

   `{PUBLIC_BASE_URL}/api/v1/accounts/google/oauth/callback` — Gmail sending accounts
   `{PUBLIC_BASE_URL}/api/auth/callback/google` — only when `AUTH_MODE=hosted` (Better Auth)

   Example: `https://openoutreach.your-subdomain.workers.dev/api/v1/accounts/google/oauth/callback`

3. Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` as Worker secrets (via `./scripts/deploy-cf.sh` or `wrangler secret put`).
4. Optionally override callback with `GOOGLE_REDIRECT_URL` if using a custom domain path.

If Google secrets are missing, OAuth start returns `oauth_not_configured` — set secrets and retry.

## Microsoft 365 / Outlook

Same vault pattern as Gmail (`CREDENTIAL_ENCRYPTION_KEY`). Provider implements `GWSClient` (`MicrosoftGraphProvider`); tick treats `provider=microsoft` like GWS for send + reply poll.

Scopes: `openid` `email` `offline_access` `Mail.Send` `Mail.Read` `User.Read`.

API:

- `POST /api/v1/accounts/microsoft/oauth/start` → `{ authorize_url }` (hidden unless `FEATURE_MICROSOFT=1` and `MICROSOFT_CLIENT_ID/SECRET` are set)
- `GET /api/v1/accounts/microsoft/oauth/callback?code&state`

Redirect URI: `{PUBLIC_BASE_URL}/api/v1/accounts/microsoft/oauth/callback`

Cloudflare Access must **bypass** that callback (Worker already treats it as a public path). `scripts/setup-cf-access.sh` creates the bypass app.

Tokens live in `microsoft_credentials` and are never returned from API/MCP.

