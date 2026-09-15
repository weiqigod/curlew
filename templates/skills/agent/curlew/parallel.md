# curlew — Parallel reference

Run with `--parallel` to execute independent requests concurrently. Curlew
computes waves from dependencies. Use `depends_on: [Exact request name]` to express
ordering; extracted-variable dependencies are also analysed. There is no YAML
`wave:` field or `parallel.workers` setting.

Preview with `run <file> --show-dependencies --dry-run`. Without `--parallel`,
requests run in document order. Within a concurrent wave, completion order is
not guaranteed. Keep dependent or stateful operations ordered explicitly.
Setup and teardown retain their phase semantics. Event `wave_index` and Markdown
wave headings describe the computed schedule; they are output, not inputs.

## Run the example

Start Mudflat using the repository's `site/README.md` setup. Save the complete
collection below as `example.yaml` in a scratch directory. `MUDFLAT_URL` defaults
to that setup's local port; override it if your fixture uses another port.

```bash
export MUDFLAT_URL="${MUDFLAT_URL:-http://127.0.0.1:18080}"
export RUN_ID="agent-$(date +%s)-$$"
curlew validate example.yaml --format json
curlew run example.yaml --var mud="$MUDFLAT_URL" --var run="$RUN_ID" --format json --parallel
```

## Complete collection

<!-- agent-source: examples/agent/parallel.yaml -->
```yaml
name: Dependency waves
requests:
- name: First
  request:
    method: GET
    url: '{{mud}}/echo'
  assertions:
    status: 200
- name: Second
  request:
    method: GET
    url: '{{mud}}/echo'
  assertions:
    status: 200
- name: After both
  depends_on:
  - First
  - Second
  request:
    method: GET
    url: '{{mud}}/echo'
  assertions:
    status: 200
```
