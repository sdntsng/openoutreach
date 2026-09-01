#!/usr/bin/env bash
# Provision Cloudflare Access apps for OpenOutreach (Allow + bypass paths) and set CF_ACCESS_AUD.
#
# Requires CLOUDFLARE_API_TOKEN with "Access: Apps and Policies Write" on the account.
# wrangler login OAuth does NOT include that scope — create a token at:
#   https://dash.cloudflare.com/profile/api-tokens
#
# Usage:
#   export CLOUDFLARE_API_TOKEN=...
#   ./scripts/setup-cf-access.sh
#
# Optional env (or set in .env.deploy):
#   PUBLIC_BASE_URL=https://openoutreach.siddhant.site
#   AUTH_ALLOWED_EMAILS=you@example.com
#   CLOUDFLARE_ACCOUNT_ID=9c8c17cfa5b8c29c830e072acce42a3d
#   --dry-run   print actions only
#   --no-secret skip wrangler secret put CF_ACCESS_AUD
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKER="$ROOT/worker"
ENV_FILE="$ROOT/.env.deploy"

DRY_RUN=0
SET_SECRET=1

usage() {
  cat <<'EOF'
Usage: ./scripts/setup-cf-access.sh [options]

Creates Cloudflare Access self-hosted applications:
  1. Main app (Allow AUTH_ALLOWED_EMAILS) on PUBLIC_BASE_URL hostname
  2. Bypass /t/*           — open tracking pixels
  3. Bypass /internal/*    — cron + health (Worker still needs X-Internal-Token)
  4. Bypass Gmail OAuth callback path

Writes CF_ACCESS_AUD to .env.deploy and runs:
  cd worker && npx wrangler secret put CF_ACCESS_AUD

Options:
  --dry-run     Print planned API calls without mutating Cloudflare
  --no-secret   Do not push CF_ACCESS_AUD to the Worker
  --env-file    Path to env file (default: .env.deploy)
  -h, --help    Show this help

Requires:
  CLOUDFLARE_API_TOKEN with Access: Apps and Policies Write
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    --no-secret) SET_SECRET=0 ;;
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

# Preserve explicit shell exports over .env.deploy (e.g. PUBLIC_BASE_URL for custom domain).
_SAVED_PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
_SAVED_AUTH_ALLOWED_EMAILS="${AUTH_ALLOWED_EMAILS:-}"
_SAVED_CLOUDFLARE_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-}"

if [[ -f "$ENV_FILE" ]]; then
  # shellcheck disable=SC1090
  set -a
  source "$ENV_FILE"
  set +a
fi

[[ -n "$_SAVED_PUBLIC_BASE_URL" ]] && PUBLIC_BASE_URL="$_SAVED_PUBLIC_BASE_URL"
[[ -n "$_SAVED_AUTH_ALLOWED_EMAILS" ]] && AUTH_ALLOWED_EMAILS="$_SAVED_AUTH_ALLOWED_EMAILS"
[[ -n "$_SAVED_CLOUDFLARE_ACCOUNT_ID" ]] && CLOUDFLARE_ACCOUNT_ID="$_SAVED_CLOUDFLARE_ACCOUNT_ID"

if [[ -z "${CLOUDFLARE_API_TOKEN:-}" && "$DRY_RUN" -eq 0 ]]; then
  cat >&2 <<'EOF'
Missing CLOUDFLARE_API_TOKEN.

Create an API token with:
  Account → Access: Apps and Policies → Edit

Then:
  export CLOUDFLARE_API_TOKEN=your_token
  ./scripts/setup-cf-access.sh

Note: `wrangler login` OAuth cannot create Access apps (auth.forbidden).
EOF
  exit 1
fi

PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-https://openoutreach.siddhant.site}"
PUBLIC_BASE_URL="${PUBLIC_BASE_URL%/}"
HOST="${PUBLIC_BASE_URL#https://}"
HOST="${HOST#http://}"

ALLOW_EMAIL="${AUTH_ALLOWED_EMAILS:-sid.tauras1@gmail.com}"
# First email when comma-separated
ALLOW_EMAIL="${ALLOW_EMAIL%%,*}"
ALLOW_EMAIL="$(echo "$ALLOW_EMAIL" | xargs)"

ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-9c8c17cfa5b8c29c830e072acce42a3d}"

export ROOT WORKER ENV_FILE DRY_RUN SET_SECRET
export CLOUDFLARE_API_TOKEN ACCOUNT_ID HOST ALLOW_EMAIL PUBLIC_BASE_URL

python3 <<'PY'
import json
import os
import sys
import urllib.error
import urllib.request

TOKEN = os.environ.get("CLOUDFLARE_API_TOKEN") or ""
ACCOUNT = os.environ["ACCOUNT_ID"]
HOST = os.environ["HOST"]
EMAIL = os.environ["ALLOW_EMAIL"]
DRY = os.environ.get("DRY_RUN") == "1"
ROOT = os.environ["ROOT"]
ENV_FILE = os.environ["ENV_FILE"]
SET_SECRET = os.environ.get("SET_SECRET") == "1"
WORKER = os.environ["WORKER"]

BASE = f"https://api.cloudflare.com/client/v4/accounts/{ACCOUNT}/access"


def api(method: str, path: str, body=None):
    url = f"{BASE}{path}"
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {TOKEN}",
            "Content-Type": "application/json",
        },
    )
    if DRY and method != "GET":
        print(f"[dry-run] {method} {path}")
        if body is not None:
            print(json.dumps(body, indent=2))
        return {"success": True, "result": {}}
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            payload = json.loads(resp.read().decode())
    except urllib.error.HTTPError as e:
        payload = e.read().decode()
        try:
            payload = json.loads(payload)
        except Exception:
            pass
        print(f"API error {method} {path}:", file=sys.stderr)
        print(json.dumps(payload, indent=2), file=sys.stderr)
        sys.exit(1)
    if not payload.get("success"):
        print(f"API failed {method} {path}:", file=sys.stderr)
        print(json.dumps(payload, indent=2), file=sys.stderr)
        sys.exit(1)
    return payload


def list_apps():
    out = api("GET", "/apps?per_page=1000")
    return out.get("result") or []


def find_app(apps, name, domain):
    for app in apps:
        if (app.get("name") or "") == name and (app.get("domain") or "") == domain:
            return app
    return None


def allow_policy():
    return {
        "name": f"Allow {EMAIL}",
        "decision": "allow",
        "include": [{"email": {"email": EMAIL}}],
    }


def bypass_policy():
    return {
        "name": "Bypass everyone",
        "decision": "bypass",
        "include": [{"everyone": {}}],
    }


def create_app(name, domain, policies):
    body = {
        "name": name,
        "domain": domain,
        "type": "self_hosted",
        "session_duration": "24h",
        "auto_redirect_to_identity": False,
        "policies": policies,
    }
    return api("POST", "/apps", body)["result"]


APP_SPECS = [
    ("OpenOutreach", HOST, "allow"),
    ("OpenOutreach bypass /t", f"{HOST}/t/*", "bypass"),
    ("OpenOutreach bypass /internal", f"{HOST}/internal/*", "bypass"),
    (
        "OpenOutreach bypass Gmail OAuth",
        f"{HOST}/api/v1/accounts/google/oauth/callback",
        "bypass",
    ),
    (
        "OpenOutreach bypass Microsoft OAuth",
        f"{HOST}/api/v1/accounts/microsoft/oauth/callback",
        "bypass",
    ),
]

print(f"==> Account {ACCOUNT}")
print(f"==> Host {HOST}")
print(f"==> Allow {EMAIL}")

apps = [] if DRY else list_apps()
main_aud = None

for name, domain, kind in APP_SPECS:
    existing = find_app(apps, name, domain) if apps else None
    if existing:
        print(f"==> Exists: {name} ({domain})")
        if kind == "allow":
            main_aud = existing.get("aud") or existing.get("application_audience")
        continue
    policies = [allow_policy()] if kind == "allow" else [bypass_policy()]
    print(f"==> Creating: {name} -> {domain}")
    created = create_app(name, domain, policies)
    if kind == "allow":
        main_aud = created.get("aud") or created.get("application_audience")
    if not DRY:
        apps.append(created)

if not main_aud:
    if DRY:
        main_aud = "dry-run-aud-placeholder"
        print("==> [dry-run] would set CF_ACCESS_AUD from main app AUD tag")
    else:
        main = find_app(list_apps(), "OpenOutreach", HOST)
        if not main:
            print("Could not find main OpenOutreach Access app", file=sys.stderr)
            sys.exit(1)
        main_aud = main.get("aud") or main.get("application_audience")
        if not main_aud:
            print("Main app has no aud tag:", json.dumps(main, indent=2), file=sys.stderr)
            sys.exit(1)

print(f"==> CF_ACCESS_AUD={main_aud}")

if DRY:
    sys.exit(0)

# Persist to .env.deploy
from pathlib import Path
import re

env_path = Path(ENV_FILE)
if env_path.exists():
    text = env_path.read_text()
    for key, val in [
        ("CF_ACCESS_AUD", main_aud),
        ("PUBLIC_BASE_URL", f"https://{HOST}"),
    ]:
        pat = re.compile(rf"^(#\s*)?{re.escape(key)}=.*$", re.M)
        if pat.search(text):
            text = pat.sub(f"{key}={val}", text, count=1)
        else:
            text = text.rstrip() + f"\n{key}={val}\n"
    env_path.write_text(text)
    print(f"==> Updated {env_path}")

# Patch wrangler PUBLIC_BASE_URL
wrangler = Path(WORKER) / "wrangler.jsonc"
if wrangler.exists():
    wtext = wrangler.read_text()
    new, n = re.subn(
        r'"PUBLIC_BASE_URL":\s*"[^"]+"',
        f'"PUBLIC_BASE_URL": "https://{HOST}"',
        wtext,
        count=1,
    )
    if n == 1:
        wrangler.write_text(new)
        print(f"==> Updated {wrangler} PUBLIC_BASE_URL")

if SET_SECRET:
    import subprocess

    print("==> wrangler secret put CF_ACCESS_AUD")
    subprocess.run(
        ["npx", "wrangler", "secret", "put", "CF_ACCESS_AUD"],
        input=main_aud.encode(),
        cwd=WORKER,
        check=True,
    )
else:
    print("==> Skipping wrangler secret (--no-secret)")

print(
    f"""
Done.

  Dashboard:  https://{HOST}/
  AUD tag:    {main_aud}

Redeploy so container env picks up PUBLIC_BASE_URL:
  ./scripts/deploy-cf.sh --skip-build

Smoke (after Access login in browser):
  curl -sS https://{HOST}/api/auth/whoami | jq
"""
)
PY
