# Development Philosophy: Always Runnable

## The Core Commitment

At every point during development, the application must run. Not "run in theory." Not "run if you set up these five things first." Actually run — accept input, do something meaningful with it, and produce output. If a developer checks out any commit on the main branch, they can build the binary and use it.

This is the single non-negotiable constraint that governs how work is sequenced, how features are introduced, and how incompleteness is handled.

## Why This Matters

A runnable application is the only honest measure of progress. Code that exists but doesn't run is inventory, not product. It carries risk — integration risk, assumption risk, the risk that what you built doesn't actually work the way you thought it would when it meets the rest of the system.

Keeping the app always runnable forces a few things that are easy to skip and expensive to recover from later. It forces integration from day one. It makes design problems visible early, when they're cheap to fix. It ensures that "done" means something — not "the function exists" but "the tool does this thing now." And it gives you a feedback loop with the actual artifact you're shipping, not a simulation of it.

## What "Runnable" Means, Precisely

A runnable application satisfies all of the following:

**It builds cleanly.** A single command produces the binary. No manual steps, no environment-specific workarounds, no "just comment out this line for now."

**It starts without errors.** Running the binary with no arguments produces help text or a sensible default message. It does not panic, crash, or emit warnings about missing configuration.

**It does what it claims to do.** Every feature the tool exposes in its help text, command list, or documentation actually works. There are no commands that exist in the interface but fail at runtime. If a command isn't ready, it isn't visible.

**It fails clearly when it should.** Invalid input produces a useful error message. Unsupported operations explain themselves. The user is never left guessing whether something went wrong or just isn't built yet.

**It can be tested by a human in under a minute.** Someone unfamiliar with the current development state can build the binary, run a basic test file, and see results. No setup guide, no database, no environment variables required for the core path.

## Vertical Slices, Not Horizontal Layers

The specification is organised by feature area: HTTP client, variable system, authentication, output formatting. This is the right way to document a product. It is the wrong way to build one under an always-runnable constraint.

Horizontal layers produce long stretches where nothing works end to end. You build a complete parser that can't execute anything. A full variable system with nothing to interpolate into. An output formatter with no results to format. Each layer is "done" in isolation, but the application doesn't work until they're all wired together — and that integration, deferred to the end, is where the surprises live.

Vertical slices cut through every layer, but thinly. Each slice delivers a narrow piece of functionality that works from input to output. The parser handles one construct. The HTTP client makes one kind of request. The output formatter renders one type of result. The slice is narrow, but it's real. You can touch it. You can run it.

The application grows by accumulating slices, not by stacking layers. At every point, it's a working tool that does fewer things than the final product — but everything it does, it does completely.

## The Completeness Contract

Every slice must honour a completeness contract before it's considered done:

**Input to output.** The slice begins with something the user provides (a file, a command, a flag) and ends with something the user sees (terminal output, a file, an exit code). If the path from input to output is incomplete, the slice isn't done.

**Error handling included.** The happy path and the sad path ship together. If the slice introduces a new input format, it also handles malformed input. If it makes an HTTP request, it handles connection failures. Error handling is not a follow-up task.

**Help text updated.** If the slice adds a command, flag, or capability, the help text reflects it. The tool's self-description is always accurate. This is a forcing function — if you can't describe the feature in help text, you may not understand the slice well enough to ship it.

**Tests that exercise the real binary.** Not just unit tests of internal functions — at least one test that builds the binary, invokes it with input, and asserts on the output. This catches integration failures that unit tests miss. The test should be something a human could reproduce manually.

## Handling What Doesn't Exist Yet

An always-runnable application has, at any given point, a large surface of things it doesn't do yet. How it handles those boundaries is a critical design decision that must be settled early and applied consistently.

### Invisible, not broken

Features that aren't built yet should be invisible, not present-but-broken. If parallel execution isn't implemented, the `--parallel` flag shouldn't exist in the CLI. If GraphQL support isn't available, the tool shouldn't parse GraphQL blocks and then fail. The absence of a feature should be silent — the tool simply doesn't offer it.

This means the CLI's surface area grows over time. Commands, flags, and supported syntax expand as slices land. The help text at any given commit is an accurate picture of what the tool can do right now.

### The feature-gate seam

The specification already defines a feature-gating system with a dedicated exit code for "this feature requires a higher tier." This same mechanism serves double duty during development. Features that are specified but not yet built can sit behind a gate that returns a clear, structured message: this capability is not available in this version.

The key distinction: gating is for features whose interface exists but whose implementation is deferred to a later tier or phase. Invisibility is for features that haven't been started at all. A feature graduates from invisible to gated to implemented.

### Graceful boundaries in file parsing

The parser will inevitably encounter constructs in user files that the current build doesn't support. A collection file might use `depends_on`, but dependency resolution isn't built yet. The tool needs a consistent strategy for this.

The recommended approach: the parser validates the full syntax it will eventually support, but the executor only handles what's implemented. If a parsed construct has no executor, the tool emits a clear error identifying the unsupported construct, the exit code that signals the limitation, and — where possible — a suggestion for how to restructure the file to avoid the unsupported feature. This way, the parser is stable from early on (reducing churn in file format support), and the executor grows incrementally.

## The Backend Boundary

The specification introduces a backend server (authentication, licensing, dashboards) starting at Phase 3. This is the most significant architectural boundary in the entire development process, because it changes what "runnable" means.

In Phases 1 and 2, "runnable" means: build the binary, run it. No external dependencies. The tool is self-contained.

From Phase 3 onward, "runnable" must account for a backend that may or may not be running. The always-runnable constraint demands that the CLI remains fully functional without the backend for all free and offline-capable features. The backend is additive — its absence degrades the experience for premium features, but never breaks the core tool.

This means the backend must itself follow the always-runnable discipline. When backend development begins, the first slice should be: a server that starts, responds to health checks, and the CLI can ping it. Every subsequent slice adds capability to both sides of the connection — the backend endpoint and the CLI code that calls it — together, as one unit.

Never build a backend endpoint without the CLI integration. Never build CLI integration against a backend that isn't running. The vertical slice spans the network boundary.

## The Sample Project

From the first commit that produces a runnable binary, a sample project should exist alongside the source code. This project is the canonical "does it work?" test. It contains collection files, test data, environment configurations — whatever the current build supports.

The sample project grows with the tool. When a slice adds variable interpolation, the sample project starts using variables. When assertions land, the sample project includes tests that pass and tests that deliberately fail. The sample project is the living documentation of what the tool can do right now.

It also serves as a regression detector. If a new slice breaks something the sample project relies on, you find out immediately — not three slices later when someone happens to test that path.

## One Thing at a Time

Under this philosophy, there is strong pressure to limit work in progress. Multiple incomplete slices are worse than one complete slice, because incomplete slices don't run. They're inventory.

The discipline is: pick a slice, complete it (input to output, errors handled, tests passing, help text updated), integrate it, verify the app still runs, then pick the next slice. Resist the urge to start the next feature before the current one is solid. The feedback loop only works if you actually use it — run the tool, see the result, confirm it works.

## What This Philosophy Does Not Prescribe

This document defines the development constraint and the principles for satisfying it. It deliberately does not prescribe:

- How slices are identified or sized (that's a planning exercise against the specification)
- Sprint length, velocity tracking, or any particular project management method
- Specific tooling for CI, testing frameworks, or build systems
- Team structure or code review processes
- Release cadence to external users

These are all valid decisions, but they're downstream of the core commitment. Get the constraint right — the app always runs — and those decisions become easier, because you have a working artifact to reason about.
