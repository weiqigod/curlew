#!/usr/bin/env bash
# replay-stripe-event.sh — signs and POSTs a canned Stripe webhook event to the local backend.
# Refs docs/SPECIFICATION.md:6796–6848 (webhook event handlers, re-fetch pattern).
#
# Usage: replay-stripe-event.sh <event-type> <subscription-id>
#
# Environment variables:
#   STRIPE_WEBHOOK_SECRET  — webhook signing secret (default: whsec_test)
#   BACKEND_URL            — base URL of the backend (default: http://localhost:5000)
#
# Examples:
#   ./scripts/replay-stripe-event.sh customer.subscription.created sub_test_1
#   ./scripts/replay-stripe-event.sh customer.subscription.deleted sub_test_1
#   ./scripts/replay-stripe-event.sh customer.updated cus_test_owner
set -euo pipefail

event_type="${1:?usage: $0 <event-type> <subscription-id-or-customer-id>}"
object_id="${2:?usage: $0 <event-type> <subscription-id-or-customer-id>}"
secret="${STRIPE_WEBHOOK_SECRET:-whsec_test}"
backend="${BACKEND_URL:-http://localhost:5000}"

evt_id="evt_replay_$(date +%s)_${RANDOM}"

# Build event object based on event type.
case "$event_type" in
  customer.subscription.*)
    evt_object=$(jq -nc \
      --arg id "$object_id" \
      '{ id: $id, object: "subscription", customer: "cus_test_owner", status: "active" }')
    ;;
  customer.updated)
    evt_object=$(jq -nc \
      --arg id "$object_id" \
      '{ id: $id, object: "customer", email: "updated@example.com" }')
    ;;
  invoice.*)
    # M14-013: invoice.payment_succeeded / invoice.payment_failed / invoice.finalized
    evt_object=$(jq -nc \
      --arg id "$object_id" \
      '{ id: $id, object: "invoice", customer: "cus_test_owner",
         subscription: "sub_test_1", status: "paid",
         amount_paid: 4900, amount_total: 4900, total: 4900, currency: "usd",
         hosted_invoice_url: ("https://invoice.stripe.test/" + $id),
         period_start: ((now - 2592000) | floor), period_end: (now | floor),
         attempt_count: 1 }')
    ;;
  payment_method.*)
    # M14-013: payment_method.attached / payment_method.detached
    evt_object=$(jq -nc \
      --arg id "$object_id" \
      '{ id: $id, object: "payment_method", customer: "cus_test_owner",
         type: "card", card: { brand: "visa", last4: "4242",
                               exp_month: 12, exp_year: 2030 } }')
    ;;
  *)
    evt_object=$(jq -nc \
      --arg id "$object_id" \
      '{ id: $id, object: "unknown" }')
    ;;
esac

body=$(jq -nc \
  --arg id "$evt_id" \
  --arg type "$event_type" \
  --argjson obj "$evt_object" \
  '{
    id: $id,
    object: "event",
    api_version: "2024-06-20",
    created: (now | floor),
    livemode: false,
    pending_webhooks: 0,
    request: null,
    type: $type,
    data: { object: $obj }
  }')

sig=$(./scripts/sign-stripe-webhook.sh "$secret" "$body")

echo "Replaying event: $evt_id ($event_type)"
curl -sS -X POST "${backend}/webhooks/stripe" \
  -H "Content-Type: application/json" \
  -H "Stripe-Signature: ${sig}" \
  --data-binary "$body"
echo
echo "Done."
