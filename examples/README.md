# Runnable examples

Run documented commands from the repository root, with `curlew` on `PATH`.
See [installation](../README.md#install) and the [agent guide](../docs/AGENT_GUIDE.md).

| Directory | What it demonstrates | Instructions |
|-----------|----------------------|--------------|
| `agent/` | Assertions, expressions, extraction, parallel requests, retries, signing and secret redaction | Each corresponding [agent skill reference](../templates/skills/agent/curlew/SKILL.md) includes the collection and exact commands; start Mudflat as described in the [cookbook setup](../site/README.md). |
| `cookbook/` | Local data-driven, faker, perf and redaction fixtures | [Cookbook setup and recipes](../site/README.md); other recipes use `testapi/collections/`. |
| `output-block/` | Inherited JSON output and a deliberate invalid-format error | [Local walkthrough](output-block/README.md). |
| `plugins/` | A JSON-RPC plugin submitting metrics to a loopback fixture | [Plugin walkthrough](plugins/datadog-metrics/README.md). |

`local-server.py` is the Python 3 fixture for the output and plugin walkthroughs.
It binds only to `127.0.0.1`. Agent and cookbook recipes use Mudflat instead.

Do not run every YAML file as a collection: `curlew.yaml` files are project
configuration, and `output-block/bad-format.yaml` must fail intentionally.
The regression suite executes the documented local recipes; real provider
accounts and their production API contracts are outside that verification.
