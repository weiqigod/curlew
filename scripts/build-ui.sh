#!/usr/bin/env bash
# Builds the apitest ui SPA into internal/uiserver/assets/dist so the next
# `go build` embeds it. Release/CI pipelines run this before building the
# binary; without it the committed placeholder index.html is served.
set -euo pipefail

cd "$(dirname "$0")/../ui"

if [ ! -d node_modules ]; then
  npm ci
fi
npm run build

echo "apitest ui assets built into internal/uiserver/assets/dist"
