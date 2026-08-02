#!/usr/bin/env bash
# seed-enterprise.sh — Seeds the enterprise fixtures required by M5-020 E2E tests.
#
# Usage:
#   ./scripts/seed-enterprise.sh <org_slug> <role_name> <perms_csv>
#
# Example:
#   ./scripts/seed-enterprise.sh acme qa-lead "results.upload,results.view,dashboard.view"
#
# Steps (all idempotent):
#   1. Mint owner JWT
#   2. Look up the org id for <org_slug> (must already exist via seed-test-data.sh)
#   3. Create an enterprise subscription (if not already present) so SSO and seats work
#   4. Create the <role_name> custom role with <perms_csv> permissions (ignore 409)
#   5. Invite qa@acme.example, accept the invite as qa, get their user id
#   6. Assign the custom role to qa via PATCH /members/{userId}
#   7. Configure SAML on the org with the fake-idp cert
#
# Env vars:
#   BACKEND_URL        (default: http://localhost:5000)
#   QA_USER_EMAIL      (default: qa@acme.example)
#   QA_USER_ID         (default: 00000000-0000-0000-0000-000000000002)
#   OWNER_USER_ID      (default: 00000000-0000-0000-0000-000000000001)
#   FAKE_IDP_HOST      (default: http://localhost:8088)  — metadata URL and SSO URL

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"
OWNER_EMAIL="${SEED_OWNER_EMAIL:-owner@example.com}"
OWNER_USER_ID="${SEED_OWNER_USER_ID:-00000000-0000-0000-0000-000000000001}"
QA_USER_EMAIL="${QA_USER_EMAIL:-qa@acme.example}"
# Fixed QA user UUID — stable across runs for idempotency.
# SYNC-NOTE: the default value is mirrored by the `SEEDED_USER_IDS` map in
# web/tests/e2e/helpers/auth.ts ('qa@acme.example' entry). If you change it
# here, change it there too — seedAuthCookie relies on the pairing to hold
# a stable JWT sub across calls.
QA_USER_ID="${QA_USER_ID:-00000000-0000-0000-0000-000000000002}"
FAKE_IDP_HOST="${FAKE_IDP_HOST:-http://localhost:8088}"

ORG_SLUG="${1:?Usage: $0 <org_slug> <role_name> <perms_csv>}"
ROLE_NAME="${2:?Usage: $0 <org_slug> <role_name> <perms_csv>}"
PERMS_CSV="${3:?Usage: $0 <org_slug> <role_name> <perms_csv>}"

# ── 1. Mint tokens ────────────────────────────────────────────────────────────
OWNER_TOKEN=$("$REPO_ROOT/scripts/test-token.sh" "$OWNER_EMAIL" "$OWNER_USER_ID")
QA_TOKEN=$("$REPO_ROOT/scripts/test-token.sh" "$QA_USER_EMAIL" "$QA_USER_ID")

# ── 2. Look up org id ─────────────────────────────────────────────────────────
ALL_ORGS=$(curl -sS "$BACKEND_URL/api/v1/organizations" \
  -H "Authorization: Bearer $OWNER_TOKEN")
ORG_ID=$(echo "$ALL_ORGS" | jq -r ".organizations[] | select(.slug == \"$ORG_SLUG\") | .id")

if [ -z "$ORG_ID" ] || [ "$ORG_ID" = "null" ]; then
  echo "ERROR: org '$ORG_SLUG' not found. Run seed-test-data.sh first." >&2
  exit 1
fi
echo "Found org '$ORG_SLUG' → $ORG_ID"

# ── 3. Ensure Enterprise subscription through the dev/test seed surface ───────
echo "Ensuring enterprise subscription and verified fixture users for org $ORG_ID..."
ORG_GUID_HEX="${ORG_ID#org_}"
ORG_GUID="${ORG_GUID_HEX:0:8}-${ORG_GUID_HEX:8:4}-${ORG_GUID_HEX:12:4}-${ORG_GUID_HEX:16:4}-${ORG_GUID_HEX:20:12}"
SUB_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$BACKEND_URL/internal/test/seed-m14" \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"org_id\":\"$ORG_GUID\",\"installation_id\":12345,\"app_id\":111,\"repos\":[\"acme/api\"],\"stripe_customer_id\":\"cus_test_owner\",\"tier\":\"enterprise\",\"verified_user_id\":\"$QA_USER_ID\",\"verified_user_email\":\"$QA_USER_EMAIL\"}")
echo "  Enterprise seed → HTTP $SUB_HTTP"
if [ "$SUB_HTTP" != "200" ]; then
  echo "ERROR: Enterprise seed returned HTTP $SUB_HTTP for org $ORG_ID" >&2
  exit 1
fi

# ── 4. Create custom role (idempotent: ignore 409) ────────────────────────────
echo "Creating custom role '$ROLE_NAME' with permissions [$PERMS_CSV]..."
# Convert comma-separated to JSON array
PERMS_JSON=$(echo "$PERMS_CSV" | tr ',' '\n' | jq -R . | jq -sc .)
ROLE_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/roles" \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"$ROLE_NAME\", \"permissions\": $PERMS_JSON}")
if [ "$ROLE_HTTP" = "201" ]; then
  echo "  Role '$ROLE_NAME' created → HTTP $ROLE_HTTP"
elif [ "$ROLE_HTTP" = "409" ]; then
  echo "  Role '$ROLE_NAME' already exists, skipping."
else
  echo "ERROR: Unexpected HTTP $ROLE_HTTP creating role '$ROLE_NAME'" >&2
  exit 1
fi

# Fetch the role id
ALL_ROLES=$(curl -sS "$BACKEND_URL/api/v1/organizations/$ORG_ID/roles" \
  -H "Authorization: Bearer $OWNER_TOKEN")
ROLE_ID=$(echo "$ALL_ROLES" | jq -r ".roles[] | select(.name == \"$ROLE_NAME\") | .id")
if [ -z "$ROLE_ID" ] || [ "$ROLE_ID" = "null" ]; then
  echo "ERROR: Could not find role '$ROLE_NAME' after creation." >&2
  exit 1
fi
echo "  Role id: $ROLE_ID"

# ── 5. Invite qa user (idempotent: skip if already a member) ─────────────────
echo "Checking if qa user is already a member of org $ORG_ID..."
MEMBERS=$(curl -sS "$BACKEND_URL/api/v1/organizations/$ORG_ID/members" \
  -H "Authorization: Bearer $OWNER_TOKEN")

# The qa user's raw GUID (no dashes)
QA_GUID=$(echo "$QA_USER_ID" | tr -d '-')

if echo "$MEMBERS" | jq -e ".members[] | select(.user_id == \"$QA_GUID\")" >/dev/null 2>&1; then
  echo "  qa user ($QA_USER_EMAIL) is already a member, skipping invite."
else
  echo "  Inviting $QA_USER_EMAIL to org $ORG_ID..."

  # POST invitation (requires subscription seat)
  INVITE_RESP=$(curl -sS \
    -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/invitations" \
    -H "Authorization: Bearer $OWNER_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$QA_USER_EMAIL\", \"role\": \"member\"}")

  INVITE_TOKEN=$(echo "$INVITE_RESP" | jq -r '.token // empty')
  if [ -z "$INVITE_TOKEN" ]; then
    echo "ERROR: Failed to create invitation: $INVITE_RESP" >&2
    exit 1
  fi
  echo "  Invitation token obtained."

  # Accept the invite as the qa user
  ACCEPT_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/invitations/accept" \
    -H "Authorization: Bearer $QA_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"token\": \"$INVITE_TOKEN\"}")
  echo "  Accept invitation → HTTP $ACCEPT_HTTP"

  if [ "$ACCEPT_HTTP" != "200" ]; then
    echo "ERROR: Invitation acceptance returned HTTP $ACCEPT_HTTP" >&2
    exit 1
  fi
fi

# ── 6. Assign custom role to qa user ─────────────────────────────────────────
# Bumps the qa user to the `admin` built-in role and layers the qa-lead custom
# role on top. The admin baseline is required so the SSO-logged-in qa user can
# read the audit log (`requireOrgAdmin` guard at /org/[slug]/audit-log) — that
# is what enterprise-full.spec.ts assertion 2 exercises. The custom role
# overlay keeps assertion 6 honest: the qa-lead row still shows
# `member_count = 1` and is_builtin = false.
echo "Assigning admin role + custom role '$ROLE_NAME' to qa user..."
PATCH_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
  -X PATCH "$BACKEND_URL/api/v1/organizations/$ORG_ID/members/$QA_GUID" \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"role\": \"admin\", \"role_id\": \"$ROLE_ID\"}")
echo "  Assign admin + custom role → HTTP $PATCH_HTTP"
if [ "$PATCH_HTTP" != "200" ]; then
  echo "ERROR: Expected 200 but got HTTP $PATCH_HTTP assigning admin + custom role '$ROLE_NAME' to qa user" >&2
  exit 1
fi

# ── 7. Configure SAML with the fake-idp cert ──────────────────────────────────
echo "Configuring SAML for org $ORG_ID..."

# Extract the org GUID from the wire format (org_<hex>) for the ACS URL
ORG_GUID_HEX="${ORG_ID#org_}"
ACS_URL="$BACKEND_URL/api/v1/sso/saml/$ORG_GUID_HEX/acs"
ENTITY_ID="$BACKEND_URL/api/v1/sso/saml/$ORG_GUID_HEX"
IDP_METADATA_URL="$FAKE_IDP_HOST/metadata"
IDP_SSO_URL="$FAKE_IDP_HOST/saml/sso"

# Read the fake-idp cert PEM (single-line JSON-safe)
CERT_PEM=$(cat "$REPO_ROOT/testdata/enterprise/fake-idp-cert.pem")

SAML_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
  -X PUT "$BACKEND_URL/api/v1/organizations/$ORG_ID/sso/saml" \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary "$(jq -n \
    --arg meta "$IDP_METADATA_URL" \
    --arg acs "$ACS_URL" \
    --arg eid "$ENTITY_ID" \
    --arg sso "$IDP_SSO_URL" \
    --arg cert "$CERT_PEM" \
    '{idp_metadata_url: $meta, acs_url: $acs, entity_id: $eid, idp_sso_url: $sso, idp_cert_pem: $cert}')")
echo "  SAML config → HTTP $SAML_HTTP"
if [ "$SAML_HTTP" != "200" ]; then
  echo "ERROR: Expected 200 but got HTTP $SAML_HTTP configuring SAML for org $ORG_ID" >&2
  exit 1
fi

echo "Enterprise seed complete."
echo "  Org: $ORG_ID"
echo "  Role: $ROLE_NAME ($ROLE_ID)"
echo "  QA user: $QA_USER_EMAIL (uid=$QA_GUID)"
