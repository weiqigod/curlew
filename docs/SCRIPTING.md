# The JS-Scripting Question

A deep look at the largest single design opinion in ApiTool: the absence of in-tool JavaScript. The assessment in `ASSESSMENT.md` treats this as the most consequential bet in the spec; this document examines it more carefully, separates the things the assessment lumped together, audits what scripting is actually used for in real test suites, and lays out a ladder of design options that aren't full JS.

The conclusion is that "no scripting" is the right call, but the framing needs work — both internally (the spec doesn't actually argue for it) and externally (the substitutes aren't yet shipped). There is a clean middle path the assessment doesn't engage with.

---

## What the spec actually says

A grep through `SPECIFICATION.md`, `DEVELOPMENT_PHILOSOPHY.md`, and `MANUAL.md` for any explicit "no scripting" or "we will not support JS" comes up empty. The four stated principles in `SPECIFICATION.md:373` are:

1. Tests as code (version-controlled files)
2. Testing-first
3. Progressive sophistication
4. AI-native design

None of these excludes scripting. Postman is "tests as code" if you commit the JSON. Karate is "testing-first." Bruno is "version-controlled files."

The "no scripting" stance is purely architectural — it's enforced by what the YAML grammar accepts, what dynamic functions exist, and what hooks the plugin system exposes. There is no document the team would have to retract if they reversed the bet. There's only a default they would be reversing.

That matters because it means **the no-scripting position is currently a fact about the implementation, not a stated commitment**. The assessment treats it as a commitment. The team should decide which it is — a fact, in which case it's revisable on its merits, or a commitment, in which case it should be written down somewhere prominent and argued for.

---

## "JS scripting" is three different things

The assessment, like most discussion of this topic, lumps three distinct designs under one label. They have different costs and different defenders.

### 1. Postman/Newman — sandboxed escape-hatch scripting

A V8 sandbox running per-request pre-scripts and test-scripts, with a constrained `pm.*` API. No `require`, no filesystem, no native modules. Each script runs in isolation; state moves between scripts via `pm.environment` and `pm.variables`.

Use shape: small scripts at the seams of declarative requests. The collection is still primarily declarative; scripts are escape hatches.

### 2. Bruno — opt-in dual mode

Bruno offers a sandboxed scripting mode similar to Postman, *and* a `node`-mode that drops the sandbox and gives users real Node.js. Two flavors, configured per project.

Use shape: same as Postman in sandbox mode. In `node`-mode, the boundary between "test definition" and "general program" disappears.

### 3. Karate — scripting as the primary medium

A JS engine (Nashorn, then Graal.js) embedded inside a Gherkin DSL. Scripting is *not* an escape hatch — it's the primary expression medium. `* def x = call read('login.feature')`, `* match response == { id: '#number' }`. Every Gherkin step can invoke JS expressions.

Use shape: the test definition itself is a small JS program with Gherkin scaffolding around it.

These should not be discussed as one design. (1) is the case ApiTool is rejecting. (3) is a different category of tool entirely — closer to Cypress or Playwright than to Postman. (2) is the worst of both worlds, because users in `node`-mode can do anything and reviewers can't tell what changed.

When the assessment writes "ApiTool refuses the embedded scripting DSL entirely," it is comparing ApiTool to (3). When it writes "Postman's V8 sandbox," it is comparing to (1). These are different bets. ApiTool is rejecting both, but the costs of allowing each are different.

---

## What scripting is actually used for

Below is a use-case audit of pre-request and test scripts as they appear in real public Postman collections, Karate suites, and Bruno repos. The "frequency" column is impressionistic — drawn from looking at the patterns in widely-shared test suites — but the categorisation is sound.

| Use case | Frequency | Solvable in ApiTool today | Solvable with planned helpers |
|---|---|---|---|
| Compute auth header (SigV4, HMAC, custom) | Very high | Partially: `from_command` (Solo), plugins (Enterprise) | Yes: helpers + free SigV4 plugin |
| Decode/inspect JWT claims | High | No | Yes: `$jwtDecode` |
| Generate dynamic body fields | High | Yes: faker, `$uuid`, `$timestamp` | Yes |
| Set env var from prior response | High | Yes: `extract:` + JSONPath | Yes |
| Branch: "if response.x then A else B" | Medium | **No** | **No** |
| Cross-field response assertions ("total equals sum of items") | Medium | Limited: multiple JSONPath asserts, no aggregation | **No** |
| Loop until condition (polling) | Medium | Yes: `retry` with condition | Yes |
| Transform extracted value (decode, parse, compute) | Medium | No | Partial: decode helpers; not arbitrary compute |
| Log to console / debug print | High | Yes: events stream, `--format json` | Yes |
| Skip request based on env / response | Medium | Partial: env files for env-based; not response-based | Same |
| Read external file at runtime | Low | Yes: `from_command`, `body_file` | Yes |
| Side effects (post to Slack, write DB) | Low | Yes: `from_command` in setup/teardown | Yes |

Two observations.

**First**, most rows are already solved or will be solved by the dynamic-function set the spec already plans. The rows that drive new users to scripting in Postman — auth headers, JWT decode, body templating — are fully covered.

**Second**, only two rows are *unsolved* even with all planned helpers: branching on response content, and cross-field aggregation assertions. These are the genuine residual.

---

## The genuine residual

### Branching on response content

The shape: "POST /orders, then if response.status is 'pending' POST /orders/:id/confirm, else POST /orders/:id/cancel."

There is no way to express this in ApiTool today. `depends_on` can sequence but not branch. `retry-until-condition` can poll but only re-runs the same request. You can fake it by running both branches and asserting one fails — but that adds noise to the report and misrepresents the system under test.

How common is this in practice? Less than the spec authors might assume. Most "branches" in real test suites are environment-based ("if prod, use TLS") and are fully handled by environment files. Response-content branches show up most in workflow-style tests — e-commerce checkout, multi-step approvals, async job submission with status polling. These tests tend to migrate off Postman to higher-level runners (Cypress, Playwright API, custom Go/Python) anyway, because at that complexity level the test is a small program, and a tool optimised for "list of requests" stops fitting.

The real risk is not losing those workflow tests — ApiTool was never going to win them. The risk is the *threshold case*: a team where 95% of tests are simple request lists and one flow needs branching. Today that team has to either fake it, split into two collections, or pick a different tool. That's where users leak out, and it's the case the no-scripting stance has to answer well.

### Cross-field aggregation

The shape: "assert that `body.total` equals the sum of `body.items[*].price`."

JSONPath has `length()`, but no `sum()`, `reduce()`, or arbitrary projection. You can write multiple per-item assertions but not a single one that compares an aggregate to a stated total.

Real-world frequency: rarer still. Most APIs that have a totals field test it via fixtures — assert that the entire response equals an expected JSON shape, where the totals are computed in the fixture, not the test logic. The pure aggregation case shows up mostly in invariants ("the response must internally agree with itself") and that's a niche.

### The summed residual

Together these two cells cover maybe 5% of test cases in a typical CI suite. Real, but bounded. They are the only cases where the answer "scripting is the right tool" is actually correct on the merits — every other row in the audit table has a better answer than scripting.

This is the number to anchor the discussion on. The question is not "is scripting useful?" — it always is, somewhere. The question is "is 5% of cases worth the cost stack below?"

---

## What full JS would actually cost

The assessment frames the cost as "principle vs paywall." That undersells the engineering cost. Real costs of embedding a JS runtime, in rough order of severity:

### 1. Diff legibility collapses on the scripted parts

The headline pitch — tests diff like config — survives only if the scripted region is small. The empirical pattern across mature Postman collections is that scripts grow until they dominate. The diff stops being readable. A reviewer can no longer answer "did this PR change behaviour?" by reading the diff; they have to mentally execute the script in their head, twice (before and after), and compare.

### 2. AI-narratability collapses

The Markdown-with-sentinels feature works because Claude can read the YAML, summarise the request, and explain the assertion in plain English. Give the agent a JS test script and it can still do that — but the value of the structured rewrite drops sharply, because the agent can read the JS source directly and a markdown summary doesn't add much over the source. The Claude-skill bet was sized for declarative input. Scripting changes the size of the bet.

### 3. Determinism breaks

A script can call `Date.now()`, `Math.random()`, `setTimeout`, `performance.now()`. Even "well-behaved" scripts use these. Faker seeding stops being a complete determinism story when scripts are in scope. The team would have to either freeze time and randomness inside the sandbox (which breaks scripts that depend on real wall clock) or accept that determinism is now best-effort.

### 4. Sandboxing is a permanent maintenance tax

V8 sandbox security is a real ongoing concern. Postman has shipped security patches for sandbox issues; any team embedding V8 inherits the responsibility to keep up with V8 CVEs and to gate every API surface (`pm.*`) as a potential escape vector. Sandbox APIs grow over time as users ask for them; each addition is a new attack surface.

### 5. `apitest validate` becomes shallow

Today `validate` catches misspelled operators, missing required fields, circular references — all without executing anything. Scripts can't be statically validated. "Lint your tests in CI" quietly weakens; the static-error-detection pitch becomes "static-error-detection except for the parts that matter most when they break."

### 6. Tier-gating becomes incoherent

Current matrix: GraphQL is Professional, plugins are Enterprise, parallel is Professional. Clean per-feature gates. With scripts: what tier is "JS that calls `fetch`"? "JS that imports `crypto`"? "JS that uses async"? Either every JS API gets gated (an enormous matrix), or it's all free (scripting becomes the cheapest way to access gated features), or there's a coarse "scripting tier" that's neither honest nor enforceable. None of those is good.

### 7. Plugin ecosystem incentive disappears

Why write a SigV4 plugin if a user can paste 30 lines of JS? The plugin model — the architecturally clean answer to "users need extension points" — gets undercut by the architecturally messy answer. The first-party plugin investment becomes harder to justify.

### 8. TDD discipline doesn't extend

`DEVELOPMENT_PHILOSOPHY.md` mandates 80% coverage and integration tests against the real binary. User-supplied scripts have none of that. The test tool is now hosting untested code, and the discipline written into the project's own documentation stops at the boundary of what the project itself ships.

### Summing the cost

These costs aren't symmetric with the benefit. Five percent of cases, bought with eight structural compromises that each weaken one of the project's stated or implied advantages. That's a bad trade. It's a much stronger argument against full JS than "it conflicts with the no-scripting principle" — especially given that the principle isn't actually written down.

---

## The middle ground

If the team ever needs to relax the stance — and the residual above is the only honest reason — there is a ladder of options long before "embed V8."

### Rung 1: `if:` on a request item

A single boolean expression using existing dynamic-function syntax:

```yaml
- name: Confirm pending order
  if: "{{response.previous.body.status == 'pending'}}"
  request:
    method: POST
    url: "{{base_url}}/orders/{{order_id}}/confirm"
```

Solves response-content branching cleanly. No new language; the expression is one already-evaluable boolean. GitHub Actions, GitLab CI, Argo Workflows all have this exact primitive. It is the smallest possible move and it covers the largest unsolved cell in the audit.

Cost: a parser change, a runtime check, a doc section. No structural concessions.

### Rung 2: a sandboxed expression language for `extract` and `assertions`

Pick one of:

- **CEL** (Common Expression Language). Kubernetes' choice — admission policies, authorisation policies. Designed exactly for "predicates and projections over JSON-shaped data." Deterministic by construction, no I/O, well-documented, multiple language implementations.
- **Starlark**. Bazel's choice. A deterministic Python subset, no I/O, no exceptions, no inheritance. Slightly more general than CEL.

Either lets you write:

```yaml
extract:
  total_cents:
    cel: "response.body.items.map(i, i.price * 100).sum()"

assertions:
  - cel: "response.body.total == response.body.items.map(i, i.price).sum()"
```

CEL especially is a good match — its design centre is "language for predicates and projections over JSON," which is exactly the residual gap. Adding CEL is also a much smaller political move than adding JS, because it doesn't read as "we caved." It reads as "we picked the right tool."

Cost: integrate a CEL evaluator (Go has at least two), define which fields accept CEL expressions, document.

### Rung 3: `transform:` chains

Compositional pipelines like jq filters:

```yaml
extract:
  user_sub:
    from: response.body.token
    transform: [jwtDecode, ".sub", trim]
```

Composable but no general computation. Doesn't solve branching but solves the "decode-then-extract" case cleanly without introducing a new language.

Cost: small. Could ship alongside the JWT decode helper as a single feature.

### Rung 4: embedded WASM plugins

The existing JSON-RPC plugin model, but in-process. Faster startup, no IPC, language-agnostic at the source level (write in Rust, Go, AssemblyScript, etc.). Higher engineering cost. Only worth doing if plugin latency becomes a real bottleneck — which, given that plugins fire per-request and per-attempt, could happen for high-iteration suites.

Cost: significant. Should not be done speculatively.

### Rung 5: full V8 / general scripting

Don't. The cost stack above applies in full.

### What the ladder achieves

Rungs 1 and 2 together — `if:` plus CEL in `extract` and `assertions` — close roughly 80% of the residual gap. Both:

- Preserve diff legibility (a CEL expression is one line; you can read it)
- Preserve AI-narratability (CEL is well-understood by LLMs trained on Kubernetes content)
- Preserve determinism (CEL has no I/O, no time, no randomness)
- Preserve static validation (`apitest validate` can parse and type-check CEL)
- Preserve clean tier-gating (`if:` and CEL are either available or not, no per-API matrix)

The cost of these two rungs together is small compared to the cost of full JS.

---

## Where the assessment was right and where it was thin

**Right.** The no-scripting stance is principled. The substitutes are *architecturally* in place — plugin host, dynamic-function registry, auth profile system, sensitivity propagation. The structural pinch point (`from_command` Solo, plugins Enterprise) is real and the recommendation that `from_command` move to the free tier and that SigV4/OAuth1/JWT/webhook-sig ship as free first-party plugins is sound.

**Thin: it framed the question as binary.** "Should ApiTool add JS scripting, yes or no?" That misses the spectrum entirely. The interesting question is "what is the smallest expressive escape from declarative YAML that closes the residual gap without breaking the pitch?" That question lives at rungs 1 and 2 of the ladder above, and the assessment doesn't engage with it.

**Thin: it treated the substitutes as already shipped.** The argument that `$hmacSha256`, `$sha256`, `$base64`, and `$urlEncode` "cover the vast majority of cases" rests on dynamic functions that don't exist in the codebase yet — they're scheduled as Phase 2/3 of the dynamic-function rollout (`SPECIFICATION.md:746-747`). Today, a free-tier user with a custom-signing requirement has neither helpers nor escape hatches (`from_command` is Solo). The defence of the no-scripting stance is forward-dated on shipping work that hasn't started.

**Thin: it underweighted the cost stack of full JS.** Treating the comparison as "principle vs paywall" misses six other costs (diff, AI, determinism, validate, gating, plugin incentives). The argument against full JS is much stronger than the assessment claims; the team can defend the position more confidently than the assessment lets them.

---

## Recommendation

**Do not add JavaScript.** The cost stack is real and the residual it would address is small.

**Do, in order:**

1. Ship the planned dynamic-function helpers (`$hmacSha256`, `$sha256`, `$base64`, `$urlEncode`, `$jsonEncode`). Without these, the no-scripting story has nothing to point at.
2. Add `$jwtDecode` as a dynamic function. Probably the highest-leverage helper not currently planned.
3. Ship SigV4 as a *free, bundled* first-party plugin (or better, as a built-in auth profile type). Move `from_command` to the free tier. These two changes together close the structural pinch point the assessment correctly identified.
4. Add an `if:` field on request items, scoped to a single boolean expression in existing interpolation syntax. Closes the largest residual cell.
5. After (1)-(4) are shipped and battle-tested, evaluate whether to add CEL or Starlark to `extract` and `assertions`. The decision will be much clearer once the rest of the substitute set is in production.

**Slogan version.** "Declarative-only" was always a hedge against *imperative* escape hatches, not against *expressions*. Letting expressions get richer doesn't break the pitch — it strengthens it, because the pitch was never really "no computation," it was "no opaque computation that breaks diffs and reasoning." CEL doesn't break either. JS does.

---

## Open questions for the team

1. Is the "no scripting" position a commitment or a fact about the current implementation? It should be one or the other; right now the docs treat it as both interchangeably.
2. Is `from_command` on the free tier a one-line change in the tier matrix, or does it have downstream implications (abuse, support burden) that haven't been thought through?
3. Are there real users today blocked on the residual (branching, cross-field aggregation), or is this a hypothetical user audit? If real users exist, what are they doing instead?
4. If CEL or Starlark were added to `extract` and `assertions`, what is the migration story for users who learned the JSONPath-only syntax? Is this an additive feature or a replacement?
5. What's the lightest-touch way to ship `if:` so that it doesn't accumulate features over time? The risk pattern is that `if:` becomes a foothold for else, elif, switch, and eventually small programs. The discipline to keep it a single boolean is part of the design.
