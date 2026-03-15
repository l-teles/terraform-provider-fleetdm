#!/usr/bin/env bash
# setup-fleet.sh – Perform Fleet first-time setup and print an API token.
#
# Usage:
#   FLEET_URL=http://localhost:8080 .github/fleet-test/setup-fleet.sh > /tmp/fleet-env
#   source /tmp/fleet-env
#
# Prints two KEY=VALUE lines to stdout for the caller to write to $GITHUB_ENV:
#   FLEETDM_URL=<url>
#   FLEETDM_API_TOKEN=<token>

set -euo pipefail

FLEET_URL="${FLEET_URL:-http://localhost:8080}"
ADMIN_EMAIL="${FLEET_ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${FLEET_ADMIN_PASSWORD:-FleetAdmin1234!}"
ADMIN_NAME="${FLEET_ADMIN_NAME:-Fleet Admin}"
ORG_NAME="${FLEET_ORG_NAME:-Test Org}"

# ---------------------------------------------------------------------------
# 1. Wait for Fleet to be healthy
# ---------------------------------------------------------------------------
echo "Waiting for Fleet at ${FLEET_URL}/healthz ..." >&2
for i in $(seq 1 60); do
  if curl -sf "${FLEET_URL}/healthz" -o /dev/null 2>&1; then
    echo "Fleet is healthy." >&2
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "ERROR: Fleet did not become healthy within 5 minutes." >&2
    exit 1
  fi
  sleep 5
done

# ---------------------------------------------------------------------------
# 2. First-time setup with retry.
#    Fleet's /healthz returns 200 as soon as the HTTP listener starts, but
#    the database may still be initialising → setup can return 500 transiently.
#    We retry up to ~60 s, accepting:
#      200 → setup succeeded
#      4xx → already configured (e.g. dev mode or prior run)
#      5xx → transient; retry after 5 s
# ---------------------------------------------------------------------------
echo "Running Fleet first-time setup..." >&2
setup_resp_file=$(mktemp)
for attempt in $(seq 1 12); do
  setup_http=$(curl -s \
    -o "$setup_resp_file" \
    -w "%{http_code}" \
    -X POST "${FLEET_URL}/api/v1/setup" \
    -H "Content-Type: application/json" \
    -d "{
      \"admin\": {
        \"email\": \"${ADMIN_EMAIL}\",
        \"name\": \"${ADMIN_NAME}\",
        \"password\": \"${ADMIN_PASSWORD}\"
      },
      \"org_info\": {\"org_name\": \"${ORG_NAME}\"},
      \"server_url\": \"${FLEET_URL}\"
    }")

  if [ "$setup_http" = "200" ]; then
    echo "Fleet setup complete (HTTP ${setup_http})." >&2
    break
  elif [[ "$setup_http" =~ ^4 ]]; then
    echo "Fleet setup returned HTTP ${setup_http}; assuming already configured." >&2
    break
  else
    echo "Fleet setup returned HTTP ${setup_http} (attempt ${attempt}/12); retrying in 5 s..." >&2
    cat "$setup_resp_file" >&2 || true
    if [ "$attempt" -eq 12 ]; then
      echo "ERROR: Fleet setup did not succeed after 12 attempts." >&2
      rm -f "$setup_resp_file"
      exit 1
    fi
    sleep 5
  fi
done
rm -f "$setup_resp_file"

# ---------------------------------------------------------------------------
# 3. Login and obtain an API token
# ---------------------------------------------------------------------------
echo "Logging in to Fleet..." >&2
login_resp=$(curl -sf -X POST "${FLEET_URL}/api/v1/fleet/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\": \"${ADMIN_EMAIL}\", \"password\": \"${ADMIN_PASSWORD}\"}")

# '.token // empty' yields "" instead of "null" when the field is absent
token=$(echo "$login_resp" | jq -r '.token // empty')

if [ -z "$token" ]; then
  echo "ERROR: Could not extract token from login response: ${login_resp}" >&2
  exit 1
fi

echo "Login successful." >&2

# ---------------------------------------------------------------------------
# 4. Emit KEY=VALUE lines for the caller to append to $GITHUB_ENV
# ---------------------------------------------------------------------------
echo "FLEETDM_URL=${FLEET_URL}"
echo "FLEETDM_API_TOKEN=${token}"
