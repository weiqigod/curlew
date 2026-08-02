#!/usr/bin/env bash
# Usage: sign-stripe-webhook.sh <secret> <body>
# Emits a Stripe-Signature header value: t=<epoch>,v1=<hex_hmac_sha256>
# Used by the M14-011 observable to test POST /webhooks/stripe manually.
set -euo pipefail
secret="${1:?usage: $0 <secret> <body>}"
body="${2:?usage: $0 <secret> <body>}"
ts=$(date +%s)
signed_payload="${ts}.${body}"
sig=$(printf %s "$signed_payload" | openssl dgst -sha256 -hmac "$secret" -hex \
        | awk '{print $NF}')
printf 't=%s,v1=%s\n' "$ts" "$sig"
