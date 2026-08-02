#!/usr/bin/env bash
# Tiny Go-based HTTP stub that accepts POST /api/v1/organizations/{org}/results
# and POST /api/v1/organizations/{org}/pr-checks, logging each call.
# Usage: ./testdata/team/mock-backend.sh
# Listens on :18080, writes request log to stdout.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec go run "$SCRIPT_DIR/mock_server.go"
