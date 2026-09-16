# Output configuration examples

Run commands from the repository root with `curlew` on `PATH` and Python 3
installed. See the [installation instructions](../../README.md#install).
These examples use the loopback fixture; no public API is required.

## Start the fixture

In a separate terminal, from the repository root:

```bash
python3 examples/local-server.py
```

The server prints `http://127.0.0.1:18081`. Stop it with Ctrl-C when finished.
If the port is busy, use `--port 0` and set `EXAMPLE_URL` in the other terminal
to the address printed by the server.

## Run it

The adjacent `curlew.yaml` supplies `output.format: json`. Curlew discovers it
from the collection's directory, even when invoked from the repository root.
JSON goes to stdout; this command saves it and checks that one request passed.

```bash
example_results=$(mktemp)
trap 'rm -f "$example_results"' EXIT
curlew run examples/output-block/collection.yaml \
  --var "base_url=${EXAMPLE_URL:-http://127.0.0.1:18081}" > "$example_results"
python3 - "$example_results" <<'PY'
import json, sys
with open(sys.argv[1]) as stream:
    result = json.load(stream)
assert result["summary"]["total"] == 1, result
assert result["summary"]["failed"] == 0, result
print("One request passed; inherited JSON output verified.")
PY
```

## Expected failure

`bad-format.yaml` deliberately uses the unsupported format `not-a-format`.
It must fail with exit code 3 before sending a request. Markdown is a supported
format and requires `--report <dir>`; it is not an unknown-format example.

```bash
example_error=$(mktemp)
trap 'rm -f "$example_error"' EXIT
if curlew run examples/output-block/bad-format.yaml \
  --var "base_url=${EXAMPLE_URL:-http://127.0.0.1:18081}" 2> "$example_error"; then
  echo "Expected exit 3, but the invalid example succeeded" >&2
  exit 1
else
  example_status=$?
fi
test "$example_status" -eq 3
python3 - "$example_error" <<'PY'
import sys
with open(sys.argv[1]) as stream:
    error = stream.read()
assert 'unknown output format "not-a-format"' in error, error
print("Expected invalid-format error verified.")
PY
```
