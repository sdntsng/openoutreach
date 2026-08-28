#!/usr/bin/env bash
# Deploy OpenOutreach to Cloudflare (Worker + Container + dashboard assets).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKER="$ROOT/worker"
ENV_FILE="$ROOT/.env.deploy"

DRY_RUN=0
SKIP_BUILD=0
NO_GENERATE=0

usage() {
  cat <<'EOF'
Usage: ./scripts/deploy-cf.sh [options]

Options:
  --dry-run       Print actions without deploying
  --skip-build    Skip web build (use existing web/dist)
  --no-generate   Do not auto-generate missing secrets
  --env-file PATH Load config from PATH (default: .env.deploy)
  -h, --help      Show this help

Setup:
  cp .env.deploy.example .env.deploy
  # Fill DATABASE_URL, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET
  ./scripts/deploy-cf.sh
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --skip-build) SKIP_BUILD=1 ;;
    --no-generate) NO_GENERATE=1 ;;
    --env-file)
      ENV_FILE="$2"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

log() { echo "==> $*"; }
warn() { echo "warning: $*" >&2; }

rand_hex() {
  openssl rand -hex 32
}

load_env() {
  if [[ ! -f "$ENV_FILE" ]]; then
    if [[ -f "$ROOT/.env.deploy.example" ]]; then
      cp "$ROOT/.env.deploy.example" "$ENV_FILE"
      log "Created $ENV_FILE from .env.deploy.example"
    else
      echo "Missing $ENV_FILE — copy from .env.deploy.example and fill in values." >&2
      exit 1
    fi
  fi
  # shellcheck disable=SC1090
  set -a
  source "$ENV_FILE"
  set +a
  STORAGE="${STORAGE:-d1}"
  STORAGE="$(echo "$STORAGE" | tr '[:upper:]' '[:lower:]')"
}

ensure_secret() {
  local name="$1"
  local val="${!name:-}"
  if [[ -n "$val" ]]; then
    return 0
  fi
  if [[ "$NO_GENERATE" -eq 1 ]]; then
    echo "Missing required value: $name (use --no-generate only after all secrets are set)" >&2
    exit 1
  fi
  val="$(rand_hex)"
  export "$name=$val"
  log "Generated $name"
  if [[ -f "$ENV_FILE" ]]; then
    python3 - "$ENV_FILE" "$name" "$val" <<'PY'
from pathlib import Path
import re, sys
path, name, val = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
text = path.read_text()
pat = re.compile(rf"^(#\s*)?{re.escape(name)}=.*$", re.M)
if pat.search(text):
    text = pat.sub(f"{name}={val}", text, count=1)
else:
    text = text.rstrip() + f"\n{name}={val}\n"
path.write_text(text)
PY
  fi
}

put_secret() {
  local name="$1"
  local val="${!name:-}"
  if [[ -z "$val" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] wrangler secret put $name"
    return 0
  fi
  log "Setting secret $name"
  printf '%s' "$val" | (cd "$WORKER" && npx wrangler secret put "$name")
}

preflight() {
  command -v node >/dev/null || { echo "node required" >&2; exit 1; }
  command -v openssl >/dev/null || { echo "openssl required" >&2; exit 1; }

  if [[ "$DRY_RUN" -eq 0 ]]; then
    if ! (cd "$WORKER" && npx wrangler whoami >/dev/null 2>&1); then
      echo "Not logged in to Cloudflare. Run: cd worker && npx wrangler login" >&2
      exit 1
    fi
  fi
}

check_storage() {
  case "$STORAGE" in
    d1)
      log "Storage: Cloudflare D1"
      ;;
    postgres)
      if [[ -z "${DATABASE_URL:-}" ]]; then
        cat >&2 <<'EOF'
DATABASE_URL is required when STORAGE=postgres.

Use a direct Postgres URL (no PgBouncer transaction pooler):

  Neon     — direct connection string (not *-pooler*)
  Supabase — port 5432 / direct mode, not transaction pooler (6543)
  Self     — must allow outbound from Cloudflare Containers

Or use STORAGE=d1 (default) for Cloudflare D1.

See docs/DEPLOYMENT.md
EOF
        exit 1
      fi
      if [[ "$DATABASE_URL" == *"pooler"* ]] || [[ "$DATABASE_URL" == *":6543"* ]]; then
        warn "DATABASE_URL looks like a pooler URL — tick/advisory locks may break."
      fi
      ;;
    *)
      echo "STORAGE must be d1 or postgres (got: $STORAGE)" >&2
      exit 1
      ;;
  esac
}

ensure_d1() {
  if [[ "$STORAGE" != "d1" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] ensure D1 database openoutreach"
    return 0
  fi
  log "Ensuring D1 database openoutreach"
  local list id
  list="$(cd "$WORKER" && npx wrangler d1 list --json 2>/dev/null || echo '[]')"
  id="$(printf '%s' "$list" | python3 -c '
import json,sys
raw=sys.stdin.read()
try:
    data=json.loads(raw)
except Exception:
    data=[]
rows=data if isinstance(data,list) else data.get("result") or data.get("databases") or []
for r in rows:
    name=r.get("name") or r.get("database_name")
    if name=="openoutreach":
        print(r.get("uuid") or r.get("id") or r.get("database_id") or "")
        break
')"
  if [[ -z "$id" ]]; then
    local out
    out="$(cd "$WORKER" && npx wrangler d1 create openoutreach 2>&1 | tee /dev/stderr)" || true
    id="$(printf '%s' "$out" | grep -Eo '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -1 || true)"
  fi
  if [[ -z "$id" ]]; then
    echo "Could not create or find D1 database openoutreach" >&2
    exit 1
  fi
  log "D1 database_id=$id"
  D1_DATABASE_ID="$id"
  export D1_DATABASE_ID
  python3 - "$WORKER/wrangler.jsonc" "$id" <<'PY'
import pathlib, re, sys
path, dbid = pathlib.Path(sys.argv[1]), sys.argv[2]
text = path.read_text()
new, n = re.subn(r'"database_id":\s*"[^"]+"', f'"database_id": "{dbid}"', text, count=1)
if n != 1:
    sys.exit("failed to patch wrangler.jsonc database_id")
path.write_text(new)
PY
}

apply_d1_migrations() {
  if [[ "$STORAGE" != "d1" ]]; then
    return 0
  fi
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] wrangler d1 migrations apply openoutreach --remote"
    return 0
  fi
  log "Applying D1 migrations"
  (cd "$WORKER" && for f in migrations/*.sql; do
    npx wrangler d1 execute openoutreach --remote --file="$f"
  done)
}

check_google() {
  if [[ -z "${GOOGLE_CLIENT_ID:-}" || -z "${GOOGLE_CLIENT_SECRET:-}" ]]; then
    warn "GOOGLE_CLIENT_ID / GOOGLE_CLIENT_SECRET not set — Google sign-in and Gmail OAuth will not work until configured."
  fi
}

deploy_worker() {
  log "Installing worker dependencies"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] cd worker && npm install --legacy-peer-deps"
  else
    (cd "$WORKER" && npm install --legacy-peer-deps)
  fi

  if [[ "$SKIP_BUILD" -eq 0 ]]; then
    log "Building dashboard"
    if [[ "$DRY_RUN" -eq 1 ]]; then
      log "[dry-run] npm run build (worker)"
    else
      (cd "$WORKER" && npm run build)
    fi
  else
    warn "Skipping web build — using existing web/dist"
  fi

  log "Deploying Worker + Container"
  local saved_base="${PUBLIC_BASE_URL:-}"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] cd worker && npx wrangler deploy"
    WORKER_URL="${PUBLIC_BASE_URL:-https://openoutreach.example.workers.dev}"
  else
    local deploy_out
    deploy_out="$(cd "$WORKER" && npx wrangler deploy 2>&1 | tee /dev/stderr)"
    local detected
    detected="$(echo "$deploy_out" | grep -Eo 'https://[a-zA-Z0-9._-]+\.workers\.dev' | head -1 || true)"
    if [[ -n "$detected" ]]; then
      # Keep explicit custom domains from .env.deploy; only default to workers.dev when unset.
      if [[ -z "$saved_base" || "$saved_base" == *".workers.dev"* ]]; then
        PUBLIC_BASE_URL="$detected"
        export PUBLIC_BASE_URL
      fi
      log "Worker URL: ${PUBLIC_BASE_URL:-$detected}"
    fi
    WORKER_URL="${PUBLIC_BASE_URL:-}"
  fi
}

write_container_env() {
  local base="${PUBLIC_BASE_URL:-}"
  local token="${INTERNAL_CONTAINER_TOKEN:-}"
  local file="$WORKER/src/container-env.ts"
  if [[ -z "$base" ]]; then
    return 0
  fi
  log "Writing container env ($base)"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "[dry-run] write $file"
    return 0
  fi
  python3 - "$file" "$base" "$token" "${STORAGE:-d1}" "${OPENOUTREACH_WORKSPACE_ID:-default}" \
    "${GOOGLE_CLIENT_ID:-}" "${GOOGLE_CLIENT_SECRET:-}" \
    "${CREDENTIAL_ENCRYPTION_KEY:-}" "${TRACKING_HMAC_SECRET:-}" \
    "${GOOGLE_REDIRECT_URL:-}" <<'PY'
from pathlib import Path
import json, sys
path, base, token, storage, workspace, google_id, google_secret, enc_key, track_secret, redirect = sys.argv[1:11]
base = base.rstrip("/")
env = {
    "LISTEN_ADDR": ":8080",
    "PUBLIC_BASE_URL": base,
    "OPENOUTREACH_WORKSPACE_ID": workspace,
}
if token:
    env["INTERNAL_CONTAINER_TOKEN"] = token
if storage == "d1":
    env["OPENOUTREACH_D1_PROXY"] = base
if google_id:
    env["GOOGLE_CLIENT_ID"] = google_id
if google_secret:
    env["GOOGLE_CLIENT_SECRET"] = google_secret
if enc_key:
    env["CREDENTIAL_ENCRYPTION_KEY"] = enc_key
if track_secret:
    env["TRACKING_HMAC_SECRET"] = track_secret
redirect = (redirect or "").strip() or f"{base}/api/v1/accounts/google/oauth/callback"
env["GOOGLE_REDIRECT_URL"] = redirect
import os
for key in (
    "FEATURE_CF_EMAIL", "FEATURE_RESEND", "FEATURE_SES", "FEATURE_WARMUP",
    "FEATURE_HUNTER", "FEATURE_MICROSOFT", "FEATURE_GMAIL", "FEATURE_SMTP_IMAP",
    "MICROSOFT_CLIENT_ID", "MICROSOFT_CLIENT_SECRET", "MICROSOFT_TENANT_ID",
    "AUTH_MODE",
):
    val = os.environ.get(key, "").strip()
    if val:
        env[key] = val
# Bumps on each deploy so warm containers pick up new env.
import time
env["CONTAINER_BOOT_REVISION"] = str(int(time.time()))
lines = [
    "/** Generated by scripts/deploy-cf.sh — do not edit by hand. */",
    "export const containerEnv: Record<string, string> = {",
]
for k, v in env.items():
    lines.append(f"  {k}: {json.dumps(v)},")
lines.append("};")
lines.append("")
Path(path).write_text("\n".join(lines))
PY
}

sync_secrets() {
  ensure_secret CREDENTIAL_ENCRYPTION_KEY
  ensure_secret INTERNAL_CONTAINER_TOKEN
  ensure_secret MCP_BEARER_TOKEN
  ensure_secret TRACKING_HMAC_SECRET
  ensure_secret BETTER_AUTH_SECRET

  if [[ -z "${PUBLIC_BASE_URL:-}" && -n "${WORKER_URL:-}" ]]; then
    PUBLIC_BASE_URL="$WORKER_URL"
    export PUBLIC_BASE_URL
  fi

  if [[ -z "${PUBLIC_BASE_URL:-}" ]]; then
    local subdomain
    subdomain="$(cd "$WORKER" && npx wrangler whoami 2>/dev/null | grep -Eo '[a-z0-9-]+\.workers\.dev' | head -1 || true)"
    if [[ -z "$subdomain" ]]; then
      # Fallback: derive from account name slug (common CF pattern)
      subdomain="siddhant-singh.workers.dev"
    fi
    PUBLIC_BASE_URL="https://openoutreach.${subdomain#https://}"
    export PUBLIC_BASE_URL
    log "Defaulting PUBLIC_BASE_URL=$PUBLIC_BASE_URL (update after deploy if subdomain differs)"
  fi

  export OPENOUTREACH_WORKSPACE_ID="${OPENOUTREACH_WORKSPACE_ID:-default}"

  local secrets=(
    CREDENTIAL_ENCRYPTION_KEY
    INTERNAL_CONTAINER_TOKEN
    MCP_BEARER_TOKEN
    TRACKING_HMAC_SECRET
    BETTER_AUTH_SECRET
  )
  if [[ "$STORAGE" == "postgres" ]]; then
    secrets+=(DATABASE_URL)
  fi
  if [[ -n "${GOOGLE_CLIENT_ID:-}" ]]; then
    secrets+=(GOOGLE_CLIENT_ID)
  fi
  if [[ -n "${GOOGLE_CLIENT_SECRET:-}" ]]; then
    secrets+=(GOOGLE_CLIENT_SECRET)
  fi
  if [[ -n "${CF_ACCESS_AUD:-}" ]]; then
    secrets+=(CF_ACCESS_AUD)
  fi
  if [[ -n "${GOOGLE_REDIRECT_URL:-}" ]]; then
    secrets+=(GOOGLE_REDIRECT_URL)
  fi
  for name in FEATURE_CF_EMAIL FEATURE_RESEND FEATURE_SES FEATURE_WARMUP FEATURE_HUNTER \
              FEATURE_MICROSOFT FEATURE_GMAIL FEATURE_SMTP_IMAP \
              MICROSOFT_CLIENT_ID MICROSOFT_CLIENT_SECRET MICROSOFT_TENANT_ID; do
    if [[ -n "${!name:-}" ]]; then
      secrets+=("$name")
    fi
  done

  for name in "${secrets[@]}"; do
    put_secret "$name"
  done
}

post_deploy() {
  local base="${PUBLIC_BASE_URL:-$WORKER_URL}"
  cat <<EOF

Deploy complete.

  Dashboard:  $base/
  Health:     $base/internal/health

Post-deploy checklist:
  1. Auth default is Cloudflare Access. Create a self-hosted Access app for this hostname,
     allow AUTH_ALLOWED_EMAILS, set CF_ACCESS_AUD, bypass /t/* /internal/* and Gmail callback.
     Or set AUTH_MODE=hosted for in-app Google/email at $base/sign-in
  2. Google OAuth redirect URI (Gmail):
     ${base}/api/v1/accounts/google/oauth/callback
  3. Microsoft OAuth redirect URI (optional):
     ${base}/api/v1/accounts/microsoft/oauth/callback
     Access bypass required (see setup-cf-access.sh).
  4. Settings → Sending Accounts → Connect Google / Microsoft / SMTP

Smoke test:
  curl -sS "$base/internal/health" -H "X-Internal-Token: \$INTERNAL_CONTAINER_TOKEN"

EOF

  if [[ "$DRY_RUN" -eq 0 && -n "$base" ]]; then
    log "Running health check"
    if curl -sfS "$base/internal/health" -H "X-Internal-Token: ${INTERNAL_CONTAINER_TOKEN:-}" >/dev/null 2>&1; then
      log "Health check OK"
    else
      warn "Health check failed (container may still be cold-starting — retry in ~30s)"
    fi
  fi
}

main() {
  preflight
  load_env
  check_storage
  check_google
  ensure_d1
  apply_d1_migrations
  sync_secrets
  write_container_env
  deploy_worker
  if [[ -n "${PUBLIC_BASE_URL:-}" && -f "$ENV_FILE" ]]; then
    # Do not clobber a custom domain with *.workers.dev from wrangler output.
    existing="$(grep -E '^PUBLIC_BASE_URL=' "$ENV_FILE" | head -1 | cut -d= -f2- || true)"
    if [[ -n "$existing" && "$existing" != *".workers.dev"* && "$PUBLIC_BASE_URL" == *".workers.dev"* ]]; then
      PUBLIC_BASE_URL="$existing"
      export PUBLIC_BASE_URL
    fi
    python3 - "$ENV_FILE" "PUBLIC_BASE_URL" "$PUBLIC_BASE_URL" <<'PY'
from pathlib import Path
import re, sys
path, name, val = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
text = path.read_text()
pat = re.compile(rf"^(#\s*)?{re.escape(name)}=.*$", re.M)
if pat.search(text):
    text = pat.sub(f"{name}={val}", text, count=1)
else:
    text = text.rstrip() + f"\n{name}={val}\n"
path.write_text(text)
PY
  fi
  post_deploy
}

main "$@"
