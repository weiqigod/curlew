// Fixture-project writer (UI_SPECIFICATION.md §13.3).
//
//   basic.yaml    — happy path, failing assertion, error endpoint,
//                   setup/teardown, big body, binary body, if:false skip
//   broken.yaml   — invalid on purpose (validation panel)
//   retried.yaml  — retry exhausts against /500 → the ↻2 badge
//   parallel.yaml — depends_on chain → waves
//
// iterations.yaml (data-driven) is deliberately not covered — recorded as a
// gap in the suite report.
//
// environments/dev.yaml carries `api_key` — the server's heuristic
// SensitiveSet (name contains "key") redacts it in GET /environments, which
// the EnvMenu renders as the [REDACTED] chip.
import * as fs from 'node:fs';
import * as path from 'node:path';

export interface FixtureOptions {
  /** Base URL of the echo server, e.g. http://127.0.0.1:49152 */
  echoUrl: string;
  /** Base URL whose port refuses connections, e.g. http://127.0.0.1:49153 */
  refusedUrl: string;
}

const CURLEW_YAML = `project_name: e2e-fixture
`;

function devEnv(opts: FixtureOptions): string {
  return `variables:
  base_url: "${opts.echoUrl}"
  refused_url: "${opts.refusedUrl}"
  api_key: "super-secret-value"
`;
}

const BASIC_YAML = `name: Basic flow
description: E2e fixture exercising every §13.3 core scenario.

setup:
  - name: Setup ping
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200

requests:
  - name: Get json ok
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200
      body:
        $.stable:
          equals: "yes"

  - name: Failing assertion
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200
      body:
        $.stable:
          equals: "nope"

  - name: Network error
    request:
      method: GET
      url: "{{refused_url}}/json"
    assertions:
      status: 200

  - name: Slow request
    request:
      method: GET
      url: "{{base_url}}/slow"
    assertions:
      status: 200

  - name: Big body
    request:
      method: GET
      url: "{{base_url}}/big"
    assertions:
      status: 200

  - name: Binary body
    request:
      method: GET
      url: "{{base_url}}/bin"
    assertions:
      status: 200

  - name: Conditionally skipped
    if: "false"
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200

teardown:
  - name: Teardown ping
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200
`;

// Invalid on purpose: the request is missing its url → the tree marks the
// collection invalid and #/file/<path> renders the validation panel.
const BROKEN_YAML = `name: Broken collection
requests:
  - name: Missing url
    request:
      method: GET
    assertions:
      status: 200
`;

// /500 always fails → retries exhaust → final 500 matches the assertion, so
// the row PASSES with attempts=3 → retry_count=2 → the ↻2 badge.
// status_codes keeps the trigger explicit; `status_ranges: ["5xx"]` would
// work too (ParseStatusRange accepts the Nxx shorthand).
const RETRIED_YAML = `name: Retried flow
requests:
  - name: Retried request
    request:
      method: GET
      url: "{{base_url}}/500"
    retry:
      enabled: true
      max_attempts: 3
      initial_delay_ms: 25
      max_delay_ms: 100
      jitter: false
      retry_on:
        status_codes: [500]
        methods: [GET]
    assertions:
      status: 500
`;

// depends_on chain → wave 1 {Create alpha, Create beta}, wave 2 {Join gamma}.
const PARALLEL_YAML = `name: Parallel flow
requests:
  - name: Create alpha
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200

  - name: Create beta
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200

  - name: Join gamma
    depends_on: [Create alpha, Create beta]
    request:
      method: GET
      url: "{{base_url}}/json"
    assertions:
      status: 200
`;

/** Writes the fixture project into `dir` (created if missing). */
export function writeFixtureProject(dir: string, opts: FixtureOptions): void {
  fs.mkdirSync(path.join(dir, 'collections'), { recursive: true });
  fs.mkdirSync(path.join(dir, 'environments'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'curlew.yaml'), CURLEW_YAML);
  fs.writeFileSync(path.join(dir, 'environments', 'dev.yaml'), devEnv(opts));
  fs.writeFileSync(path.join(dir, 'collections', 'basic.yaml'), BASIC_YAML);
  fs.writeFileSync(path.join(dir, 'collections', 'broken.yaml'), BROKEN_YAML);
  fs.writeFileSync(path.join(dir, 'collections', 'retried.yaml'), RETRIED_YAML);
  fs.writeFileSync(path.join(dir, 'collections', 'parallel.yaml'), PARALLEL_YAML);
}
