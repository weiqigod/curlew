# Assessment: Curlew Among Its Peers

A read of the specification, manual, development philosophy, and tech-choices documents, placed against the existing tools in this category. The goal is to name where Curlew is genuinely opinionated, where it follows the field, and where the strongest design bets sit — including the most consequential one, the no-scripting stance.

---

## What the project is

Curlew is a file-based HTTP testing tool. The artifact is a single static Go binary (`curlew`) that runs YAML collections; a C#/.NET backend joins in Phase 3 for licensing, the web dashboard, and team-tier coordination. It positions itself as the thing you reach for when Postman starts fighting you — diffs instead of workspaces, pull requests instead of shared accounts, no telemetry, no GUI.

The feature surface is broad: HTTP, GraphQL, and WebSocket protocols, JSONPath assertions plus JSON Schema validation, a 10-level variable precedence ladder, dynamic functions and 53 faker functions with seeded determinism (en-US only — locale support spec'd at `SPECIFICATION.md:859–904` but deferred), vault integration that shells out to provider CLIs (AWS, Azure, HashiCorp, GCP, 1Password), retry with multiple backoff strategies, data-driven iteration, parallel execution via dependency analysis, OpenAPI import, six output formats (terminal, JSON, TAP, JUnit, HTML, Markdown), JSONL logging, an event stream, a process-level plugin model over JSON-RPC stdio, built-in request signers (AWS SigV4, OAuth 1.0a) for the canonical custom-signing cases, and Phase-5 distributed performance workers. Five tiers gate the surface (Free, Solo $9, Professional $19, Team $39, Enterprise). The development discipline is mandated TDD on always-runnable vertical slices, with `ci-local.sh` as the authoritative gate.

---

## Where it sits next to peers

The closest peers are Hurl, Bruno, Stepci, Newman/Postman, Insomnia/Inso, Karate, the VS Code REST Client, and at the load-test edge, k6.

Compared to Hurl (free, plain-text DSL, minimalist), Curlew is far more featured but commercial. Hurl is the most ergonomically similar tool and stops at solo-developer scope; Curlew extends into team and enterprise.

Compared to Bruno (file-based, GUI-forward, custom `.bru` format), Curlew is CLI-forward, picks YAML over a custom format, and has no GUI ambitions. Bruno monetises cloud sync; Curlew monetises CLI capabilities.

Compared to Postman/Newman the gap is stark: no GUI, no JavaScript pre/post-request scripts, no shared workspaces. The "tests-as-code, run in CI" stance is what Postman has been retrofitting for years.

Compared to Karate, Curlew refuses the embedded scripting DSL entirely — Karate gives you Gherkin plus JS; Curlew gives you declarative YAML plus `from_command` and plugins for everything that would have been a script.

Compared to Stepci, the closest ideological cousin (YAML, CLI, OSS), Curlew is wider in scope and explicitly commercial.

On a low-opinion-to-high-opinion axis, Hurl and the VS Code REST Client sit at the minimalist end (one file format, few knobs, nothing to argue with), Postman and Karate sit at the maximalist end (huge feature set, scripting, many ways to do anything), and Bruno/Stepci occupy the middle. Curlew is closer to the maximalist end on feature breadth but pairs that breadth with a much more constrained execution model than Postman or Karate. Effectively: as much capability as the heavyweight tools, delivered through a deliberately narrow declarative surface and a strict CLI/CI ergonomics contract.

The closest one-line positioning: it's what you'd build if you took Hurl's discipline, gave it Postman's feature checklist, removed all the scripting, and explicitly designed the output for an AI agent to read.

---

## How opinionated it is

By the standards of this category, Curlew is unusually opinionated — not in a quirky way, but in the sense that several large design decisions are settled and enforced rather than left as configuration.

**Declarative-only execution.** No scripting hooks. Pre and post-request logic happens through `from_command`, dynamic variables, vault profiles, or external plugins. This is a real divergence from Postman, Bruno, Karate, and Insomnia, all of which let you drop into a JS or JVM context. Treated separately below.

**YAML, not a custom DSL or JSON.** Hurl picked plaintext, Postman picked JSON, Bruno picked `.bru`, REST Client picked `.http`. Curlew commits to YAML and ships JSON Schemas (`curlew schema`) so editor support is a first-class feature. The opinion: this format is for humans editing in pull requests, not for round-tripping through a UI.

**Stream discipline as a contract.** stdout carries only the `--format` payload; stderr carries everything else, regardless of TTY state. The exit-code table is small, numbered, and precisely scoped (with exit 6 reserved for tier gating). This is more rigour than most peers — Newman and Postman in particular are loose about this.

**The Markdown output format and the sentinel-bracketed splice.** This is the most distinctive feature in the spec, and I haven't seen an equivalent elsewhere. `--format markdown --report dir/` writes one file per request with a fixed ten-section CLI-owned region between HTML-comment sentinels, and rewrites only those bytes on re-run — agent or human notes outside the sentinels survive byte-for-byte. Combined with the bundled `curlew init --skill claude` and the `exec --stdin --format json` agent affordances, the design treats AI agents as a co-equal user class, not an afterthought.

**Sensitivity as a one-way ratchet.** Once a value is sensitive, propagation through interpolation keeps it sensitive, and an explicit `sensitive: false` on an inherited sensitive variable is a hard error. Auth profile variables are auto-sensitive with no opt-out. Most peers redact at the leaf and stop there.

**Plugins as external JSON-RPC processes.** No embedded sandbox, no shared-library ABI. Any language that can read stdin and write stdout qualifies. This is more like LSP or MCP than like Postman's V8 sandbox — pragmatic, language-agnostic, OS-sandboxable, and deliberately less powerful than in-process scripting.

**Monetisation aimed at CLI capabilities, not cloud sync.** Solo unlocks vault integration, dynamic auth, retry. Professional unlocks parallel, data-driven, GraphQL, WebSocket, HTML reports, glob discovery, JSON Schema body assertions, OpenAPI import. Team adds shared vault templates and PR checks. Enterprise adds plugins, distributed workers, and `perf`. Most competitors monetise the collaborative cloud (Postman, Bruno, Insomnia); Curlew monetises depth of CLI behavior. That's a different bet about who pays and why.

**Free-tier 1,000-request guard rail with its own exit code (2).** The spec is explicit that this is abuse prevention, not a conversion mechanism. Still, it's an opinion most peers don't take.

**Engineering discipline written into the project.** Mandatory TDD, always-runnable vertical slices, "invisible-not-broken" feature handling, the completeness contract per slice (input to output, error handling, help text, integration test against the real binary), 80% coverage as a hard floor, every commit on a feature branch. This isn't unusual to *believe in*; it's unusual to *enforce* this rigidly in the project's own docs.

---

## The no-scripting bet

This deserves its own treatment because it's the largest single opinion in the design and the one most likely to determine adoption.

### The case for it

Scripting is where most Postman collections in the wild become unreviewable. A pre-request script is a 30-line JS blob that diffs say nothing useful about, runs in a sandbox with its own quirks, and quietly mutates state that the next request depends on. By the time you have a few of those, the collection is no longer a test — it's a small program that happens to live in a JSON file. Curlew's pitch is that tests should diff like configuration, not like code, and that pitch is incompatible with arbitrary scripting.

The same property pays off twice. A declarative collection is also legible to an AI agent and to a code reviewer, which feeds directly into the markdown-output and Claude-skill bets elsewhere in the design. So the philosophical move is coherent — declarative-only, AI-narratable, version-controllable, deterministic. Pick three; you get four.

### The escape hatches are better than the slogan suggests

`$hmacSha256`, `$sha256`, `$base64`, `$urlEncode`, `$randomHex`, and the rest cover the vast majority of "I need to compute a header" cases without leaving YAML. Nested interpolation means `Authorization: "{{$hmacSha256('{{user_id}}:{{ts}}', '{{secret}}')}}"` actually works. Dynamic auth profiles handle the "log in, capture token, use it" loop that's probably the single most common reason people reach for scripts in Postman. Setup/teardown plus extraction plus `depends_on` covers stateful flows. Retry-with-condition handles the polling cases that aren't truly arbitrary loops.

So the surface that's *actually* unreachable from declarative plus dynamic functions is narrower than it sounds — it's mostly custom signing schemes, branching on response content, and response-body transformations that don't fit JSONPath extraction.

### Where the bet starts to wobble

Three specific concerns.

**The named escape hatches are tier-gated.** `from_command` is Solo. Plugins are Enterprise. So when a free-tier user hits the wall, the answer "use the escape hatch" is also "pay us." That's a structural pinch point Hurl doesn't have (escape via shell-wrapping is free) and that Bruno doesn't have (just write the script). It means "no scripting" isn't only a design opinion — it's also load-bearing for the monetization model, and the tension between those two roles is real. If the dynamic-function and faker surfaces don't grow fast enough to keep the free tier feeling complete, the no-scripting story stops reading as principled and starts reading as artificial scarcity. That's the version of this bet that ages badly.

**Portability of `from_command`.** It runs through `/bin/sh -c` per the spec. That's fine on macOS and Linux, awkward on Windows. A tool that prides itself on a single static binary that "just runs" hands a real footgun to Windows users who need exactly the workflow that `from_command` is supposed to absorb. Worth a closer look — either standardize on a portable invocation (`cmd /c` fallback, or a small embedded shell), or document the Windows story very clearly.

**The historical pattern.** Every declarative testing or automation system I can think of grew scripting eventually. Helm bolted on Sprig. Ansible has Jinja2 plus Python plugins. Terraform has HCL plus provisioner exec. CircleCI added orbs. The pressure is monotonic — every team eventually has *one* workflow that doesn't fit, and the cost of saying "no" is losing that team to a tool that says "yes." The plugin model is Curlew's pre-emptive answer to this, and it's a good one architecturally, but it depends entirely on plugins actually existing. An empty plugin ecosystem behind an Enterprise gate is a thin wall against that pressure. The spec hints at this with `curlew-sigv4` as the `on_request` example, which suggests the team knows custom signing is the canonical "I need scripting" case — but custom signing also needs to ship as a built-in or as a free first-party plugin, not as something you have to write yourself or pay Enterprise to load.

### Verdict on no-scripting

The design opinion is correct, the substitute affordances are surprisingly capable, and the bet would land cleanly if two things changed.

First, the escape hatches should be free or near-free. `from_command` in particular feels like it should be the safety valve everyone has access to, not a $9/month paywall.

Second, the first-party plugin set needs to ship populated, not as an architectural promise. SigV4, OAuth1, JWT decode, and webhook signature verification at minimum, available without an Enterprise license, before this story is complete. **Resolved by M17** (with rescoped framing — these ship as built-in signers and dynamic functions rather than plugin binaries, available across all tiers; functionally equivalent for the no-scripting argument).

If both of those happen, "no scripting" reads as a strong, defensible opinion that competitors will struggle to match because they can't unbuild their scripting layers. If neither happens, it reads as a paywall dressed up as a principle, and the first competitor that fixes the same review-and-determinism problems *with* scripting will eat into the niche from the side Curlew can't defend.

One-line take: the bet is sound, but the team's job for the next year is making sure nobody ever has to want scripting badly enough to leave — and that's a content problem (faker breadth, dynamic-function coverage, first-party plugins, free-tier escape valves) more than an architecture problem.

---

## Overall verdict

Curlew is a coherent, opinionated, AI-aware take on a crowded category. The breadth is comparable to the heavyweights (Postman, Karate); the discipline is closer to the minimalists (Hurl). The two genuinely novel bets are the agent-native markdown output with sentinel-preserved notes, which no major competitor has staked a claim on, and the no-scripting stance, which is principled but commercially load-bearing in a way the docs don't fully reckon with. The engineering rigour written into `DEVELOPMENT_PHILOSOPHY.md` and `TECH_CHOICES.md` is the project's structural advantage and its insurance policy: as long as the always-runnable, vertical-slice, completeness-contract discipline holds, the surface will fill in evenly and the gaps in the no-scripting story will close before they become exit ramps for users.

---

## What would close the gaps in this assessment

The piece above is a first read. A second pass against the actual codebase and the planned-but-unshipped feature set surfaced specific things that should be fixed before this document is treated as complete. They divide cleanly into factual errors, framing problems, structural issues, and missing dimensions. Tone is its own question and is treated last.

### Factual errors

The most consequential is that **the named escape-hatch dynamic functions don't exist yet**. The text above says `$hmacSha256, $sha256, $base64, $urlEncode, $randomHex, and the rest cover the vast majority of "I need to compute a header" cases.` Only `$randomHex` is actually registered in `internal/variable/dynamic.go`. The others are scheduled in the spec as Phase 2/3 of the dynamic-function rollout (`SPECIFICATION.md:746-747`) — designed but not implemented. The whole "the substitute affordances are surprisingly capable" argument leans on functions that aren't there yet, which weakens the no-scripting defense in the present tense. Either the prose should shift to "would cover, once shipped" or it should explicitly note that this part of the defense is forward-dated on engineering work that hasn't started. **(Resolved: M12 shipped all 13 named functions including `$base64`, `$hmacSha256`, `$urlEncode`, `$sha256`, `$md5`, `$jsonEncode`, `$base64Decode`, `$dateAdd`, `$dateSubtract`, `$formatDate`, `$parseDate`, `$randomPassword`, `$randomBase64`. M13 added 53 faker functions. M17 added webhook-signature helpers and JWT decode. The escape-hatch claim now stands on actual code.)**

The **tier-gating critique is also incomplete**. The text names `from_command` (Solo) and plugins (Enterprise) as the structural pinch points where "no scripting" doubles as paywall. Missing from that list: dynamic auth profiles are also Solo, retry is Solo, parallel is Professional, GraphQL is Professional, JSON Schema body assertion is Professional. The free tier is meaningfully thinner than the assessment lets on, and the line about "if the dynamic-function and faker surfaces don't grow fast enough to keep the free tier feeling complete" undersells how much is already paywalled today.

A subtler one: the **engineering-rigour praise is unverified**. The closing paragraph treats mandatory TDD, 80% coverage, and the completeness contract as the project's structural advantage. But these are aspirational documents. The assessment doesn't sample the codebase to check whether actual coverage hits 80%, whether the commit history reflects the vertical-slice discipline, or whether `ci-local.sh` gates what the docs say it gates. For a defensible verdict, this should be at least spot-checked.

### Framing problems

The largest framing problem is that **the scripting question is presented as binary**. "Add scripting yes/no" misses the spectrum — `if:` fields, sandboxed expression languages like CEL or Starlark, `transform:` chains — all sit between "no scripting" and "embed V8" and are not engaged with at all. A separate document (`docs/SCRIPTING.md`) walks that ladder; the assessment should at minimum reference it, and arguably should fold its conclusions in. The current treatment makes the bet look more all-or-nothing than it actually is.

**Karate and Postman are conflated**. The piece treats both as "embedded scripting," but Karate's JS is the *primary* expression medium (test definitions are small JS programs with Gherkin scaffolding), while Postman's is *escape-hatch* scripting (mostly-declarative collections with JS at the seams). Curlew is rejecting both, but it's rejecting different things, and the costs of allowing each differ enough that they should be split.

**The Hurl comparison is thin**. Hurl is named as the "minimalist" peer, but the piece doesn't engage with how Hurl's captures-and-asserts syntax compares to Curlew's planned dynamic-function set. Hurl already has JSONPath, regex, and XPath capture, plus assertion predicates that overlap meaningfully with what Curlew plans. The current comparison says "Hurl is simpler, Curlew is featured-er" — true but shallow.

### Structural problems

**The Markdown sentinel-splice deserves its own section**. It's flagged as "the most distinctive feature in the spec, and I haven't seen an equivalent elsewhere" — a strong claim that currently gets one paragraph inside "How opinionated it is." The no-scripting bet, by contrast, gets a multi-page treatment. The asymmetry suggests the author found the no-scripting question more interesting; a complete assessment should give the agent-affordance bet equivalent depth — verification that the claim of novelty holds against current peer tools, an honest examination of when sentinel-preserved notes actually matter, and a question about whether the bet survives if Claude or competitor models get materially better at structured output natively.

The **C# backend, dashboard, and team features get one sentence**. The opening flags Phase 3 but the body is entirely about the CLI. The licensing model, the dashboard, the PR-checks integration, the team-tier coordination — none of these is analyzed. If the assessment is meant to cover the whole project, this is a gap. If it's intentionally CLI-only, the scope should be stated up front.

**The recommendations are buried**. The two actionable recommendations (`from_command` should move to the free tier; the first-party plugin set should ship populated, not promised) are inside the no-scripting subsection. A document titled "Assessment" that someone might read to make decisions should surface them — either as a final "What to do" section, a top-of-document executive summary, or both.

### Missing dimensions

**Distribution and packaging is under-weighted**. "Single static Go binary" appears once. Against Postman and Bruno (Electron-based, ~200MB, separate installers per OS), and against Hurl (also a single binary, but with a narrower feature surface), this is one of the project's stronger competitive levers and the piece treats it as a stylistic note rather than a meaningful advantage.

**Telemetry and trust posture is a one-liner**. Postman has had explicit telemetry-driven controversies (the 2023 forced-cloud-sync incident drove a measurable migration to Bruno and Insomnia). "No telemetry" is more of a competitive lever in this market than the piece treats it as.

**Pricing isn't benchmarked**. Five tiers with prices ($9 Solo, $19 Professional, $39 Team) are listed but never compared to peers. Postman is $19/seat for Pro, Bruno is $9 for Pro, Insomnia is $5/$12. Whether Curlew is priced right, and whether the Solo→Professional jump (a 2x price step for parallel + GraphQL + WebSocket + HTML reports + glob discovery + JSON Schema + OpenAPI import) reflects the right value gradient, are questions the assessment is silent on.

### Tone

The verdict is gentle. "Coherent, opinionated, AI-aware" is a warm wrap. A complete assessment should also name failure modes — not just the no-scripting bet, but ecosystem traction (will plugins actually exist?), Windows portability beyond `from_command` (the file-watcher, the sentinel-rewrite, terminal output), the support cost of a "no GUI" tool for the non-developers on teams that buy Team-tier licenses, and the maintenance burden of five tiers. The ending currently reads as "this is good and will probably work"; a sharper version names what would have to go wrong, and how to know early.

### Suggested order of attack

If the goal is a defensible v1 of this document:

1. **Factual pass.** Fix the dynamic-function escape-hatch claim, broaden the tier-gating critique to the full matrix, and verify the engineering-rigour claims against the actual codebase. These are the items that, if left unaddressed, undermine the credibility of everything else.
2. **Structural pass.** Split the Karate/Postman conflation, give the markdown-sentinel bet its own section parallel to the no-scripting bet, fold in or reference the scripting-ladder argument from `SCRIPTING.md`, and lift the recommendations out of the no-scripting subsection into a dedicated "What to do" closing.
3. **Broadening pass.** Add the missing dimensions: C#/dashboard/team features, distribution as a competitive lever, telemetry as a competitive lever, pricing benchmarked against peers. This is the largest content lift and should come last because it depends on the structure being right first.

Tone is independent of all three passes and should be decided once the audience is settled — sharper for the team, gentler for an external reader.
