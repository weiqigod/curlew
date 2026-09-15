#!/usr/bin/env bash
# Builds the curlew ui SPA into internal/uiserver/assets/dist so the next
# `go build` embeds it. Release/CI pipelines run this before building the
# binary; without it the committed placeholder index.html is served.
set -euo pipefail

cd "$(dirname "$0")/../ui"

npm ci
npm run build

echo "curlew ui assets built into internal/uiserver/assets/dist"
