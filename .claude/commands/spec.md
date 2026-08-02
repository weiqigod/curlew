Look up specification details for: $ARGUMENTS

---

## Step 1: Map Topic to Section

The specification is `docs/SPECIFICATION.md` (~340KB). **Do NOT read the entire file.** Use targeted searches to find the relevant section.

### Topic-to-File Mapping

| Topic Keywords | Primary Section | Secondary |
|---------------|----------------|-----------|
| `collection`, `yaml`, `file format`, `schema` | Collection file format | Variable system |
| `request`, `http`, `methods`, `headers`, `body` | Request execution | Collection format |
| `variable`, `interpolation`, `env`, `${...}` | Variable system | Collection format |
| `assertion`, `test`, `check`, `expect` | Assertions | Output formatting |
| `output`, `format`, `terminal`, `json`, `tap` | Output formatting | CLI interface |
| `auth`, `license`, `tier`, `gate`, `token` | Authentication & licensing | CLI interface |
| `cli`, `command`, `flag`, `args`, `subcommand` | CLI interface | Output formatting |
| `ai`, `agent`, `machine`, `generate` | AI/agent integration | CLI interface |
| `error`, `exit code`, `failure` | Error handling & exit codes | Assertions |
| `parallel`, `concurrent`, `batch` | Parallel execution | Request execution |
| `chain`, `extract`, `response`, `capture` | Response chaining | Variable system |
| `environment`, `config`, `settings` | Environment configuration | Variable system |
| `milestone`, `phase`, `roadmap` | Milestones & phases | — |

---

## Step 2: Extract Details

Read the matched section(s) now. Do NOT work from memory.

From the matched section(s), extract and organize:

1. **Requirements** — statements with MUST, SHOULD, MAY (quote exactly)
2. **Data Structures** — YAML schemas, type definitions, field descriptions
3. **Algorithms** — Processing rules, resolution order, precedence
4. **Edge Cases** — Boundary conditions, error states, empty inputs
5. **Error Conditions** — What fails, how it fails, exit codes
6. **Examples** — Code samples, YAML snippets, CLI invocations

---

## Step 3: Cross-reference

Check related sections for requirements that affect the topic. For example:
- Variable interpolation rules affect request execution, assertions, and output
- CLI interface requirements affect how every feature is exposed
- Error handling conventions apply across all features
- Output formatting affects how every result is displayed

---

## Step 4: Summarize

Present findings using this structure:

```markdown
## Specification: <Topic>

### Requirements
- **MUST**: ...
- **SHOULD**: ...
- **MAY**: ...

### Data Structures
<schemas, types, fields>

### Processing Rules
<algorithms, resolution order, precedence>

### Edge Cases
<boundary conditions, empty inputs, limits>

### Error Conditions
<what fails, error messages, exit codes>

### Examples
<code samples, YAML snippets>

### Cross-references
<related requirements from other sections>
```

---

## Common Topics Quick Reference

These summaries help orient you, but **always read the actual specification** for authoritative details:

- **Collection Format**: YAML-based `.api` files defining requests, variables, assertions. Supports single-request and multi-request collections.
- **Request Execution**: HTTP method, URL, headers, body. Supports all standard methods. Handles redirects, timeouts, TLS.
- **Variable System**: `${var}` interpolation. Resolution order: CLI flags → environment → file-level → collection-level. Supports extraction from responses.
- **Assertions**: Status code, header, body, JSON path, response time checks. Multiple assertion types per request.
- **Output Formats**: Terminal (human), JSON (machine), TAP (CI). Verbosity levels. Color support.
- **CLI Interface**: `curlew run <file>` as primary command. Flags for environment, variables, output format, verbosity.
- **Error Handling**: Structured error types, meaningful messages, appropriate exit codes (0 = success, 1 = assertion failure, 2 = execution error).
- **Milestones**: M1 = CLI tool (Go), M2 = advanced features, M3 = backend (C#).

## Notes

- If the topic is ambiguous, list the possible section matches and ask for clarification.
- If the specification does not cover the topic, say so explicitly — do not invent requirements.
- When quoting requirements, include the exact MUST/SHOULD/MAY language.
