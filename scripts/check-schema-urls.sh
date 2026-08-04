#!/usr/bin/env bash
# Verify that the $id of each published JSON Schema resolves over HTTP.
#
# MANUAL §1.5 hands these URLs to users working outside the source tree, so a
# wrong org segment or a renamed default branch turns editor setup into a 404.
# The Go test TestSchema_ids_match_module_path pins the string; only a real
# request can confirm it resolves, so this lives here rather than in `go test`.
#
# Network-dependent by design: not part of ./scripts/ci-local.sh. Run it after
# pushing a change to schemas/*.json, and after any repository rename.
#
#   ./scripts/check-schema-urls.sh
#
# Exits non-zero if any $id does not return 200.
set -euo pipefail

cd "$(dirname "$0")/.."

status=0
for file in schemas/*.json; do
    url=$(python3 -c "import json,sys; print(json.load(open('$file')).get('\$id',''))")
    if [ -z "$url" ]; then
        echo "FAIL  $file declares no \$id"
        status=1
        continue
    fi
    code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 15 "$url" || echo "000")
    if [ "$code" = "200" ]; then
        echo "ok    $file -> $url"
    else
        echo "FAIL  $file -> $url returned HTTP $code"
        echo "      A private repository also returns 404 here. If that is expected,"
        echo "      MANUAL §1.5 must say so rather than offering the URL."
        status=1
    fi
done

exit $status
