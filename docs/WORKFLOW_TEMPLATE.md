# Claude Code Workflow Template

A complete, reusable template for setting up a TDD-driven, task-managed development workflow with Claude Code. This document describes the workflow system used in KnowledgeDb2, generalized so it can be bootstrapped into any new project.

**To use this template:** Give this document to Claude Code in a new project and say:
> "Set up this workflow for my project. My project is [description], using [language/framework]."

Claude Code will adapt the structure, commands, and conventions to fit your project.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Directory Structure](#2-directory-structure)
3. [CLAUDE.md Template](#3-claudemd-template)
4. [Management Files](#4-management-files)
5. [Slash Commands](#5-slash-commands)
6. [Skills](#6-skills)
7. [Task Lifecycle](#7-task-lifecycle)
8. [Adaptation Guide](#8-adaptation-guide)

---

## 1. Overview

### Philosophy

- **Test-Driven Development (TDD)** — Every feature starts with a failing test
- **Vertical Slices** — Each task delivers observable, runnable output
- **Artifact-Driven** — Plans, reviews, improvements, and verifications are tracked as files
- **Quality Gates** — Code must pass build, test, format, and review before merging

### Workflow Pipeline

```
/backlog <milestone>  →  Generate task definitions from roadmap
/plan <TASK-ID>       →  Create implementation plan (status: planned)
/execute <TASK-ID>    →  TDD implementation (status: in_progress → review)
/review <TASK-ID>     →  Code quality audit (verdict: PASS/FAIL)
/improve <TASK-ID>    →  Fix review findings (if FAIL)
/verify <TASK-ID>     →  Final verification, PR, merge (status: done)
```

### Supporting Commands

```
/status               →  Project dashboard
/test <scope>         →  Write tests for a component
/commit               →  Prepare conventional commit
/spec <topic>         →  Look up specification details
```

---

## 2. Directory Structure

```
<project-root>/
├── CLAUDE.md                           # Project instructions for Claude Code
├── CHANGELOG.md                        # Release changelog
├── .claude/
│   ├── settings.local.json             # Permissions config
│   ├── commands/                       # Slash commands (one .md per command)
│   │   ├── plan.md
│   │   ├── execute.md
│   │   ├── review.md
│   │   ├── improve.md
│   │   ├── verify.md
│   │   ├── test.md
│   │   ├── commit.md
│   │   └── spec.md
│   └── skills/                         # Skills (each in its own folder)
│       ├── backlog/
│       │   ├── SKILL.md
│       │   ├── task-template.md
│       │   └── milestone-mapping.md
│       └── status/
│           └── SKILL.md
├── management/
│   ├── backlog.yaml                    # Task index with statuses
│   ├── tasks/                          # Individual task YAML definitions
│   │   ├── <PREFIX>-001.yaml
│   │   └── ...
│   ├── plans/                          # Implementation plans, verification & improvement reports
│   │   ├── <TASK-ID>-plan.md
│   │   ├── <TASK-ID>-verified.md
│   │   └── <TASK-ID>-improved.md
│   └── reviews/                        # Code review reports
│       └── <TASK-ID>-review.md
└── docs/
    ├── SPECIFICATION.md                # Full system specification (optional but recommended)
    ├── DEVELOPMENT_ROADMAP.md          # Milestone roadmap (optional but recommended)
    └── ARCHITECTURE_DECISIONS.md       # ADRs (optional)
```

---

## 3. CLAUDE.md Template

The CLAUDE.md file is the primary instruction file for Claude Code. Adapt this template to your project.

````markdown
# CLAUDE.md

## Project Overview

<PROJECT_NAME> is <brief description>.

**Language:** <language / framework>
**Current Status:** <Pre-implementation | In progress>

## Key Documentation

- **docs/SPECIFICATION.md** - Full system specification
- **docs/DEVELOPMENT_ROADMAP.md** - Development roadmap with milestones

## Development Philosophy

### Test-Driven Development (Mandatory)

**All development MUST follow TDD. No exceptions.**

1. **RED**: Write a failing test first that defines expected behavior
2. **GREEN**: Write the minimum code to make the test pass
3. **REFACTOR**: Improve code quality while keeping tests green

**Never write implementation code without a failing test first.**

### Vertical Slice Approach

**Every task delivers observable, runnable output.** Tasks are organized by capability
(what the user can do), not by technical layer.

#### Core Principles

1. **Observable Output**: Every task produces something you can run and see
2. **TDD Path**: Behaviors → Tests → Implementation → Observable
3. **Incremental Value**: Each slice builds on previous, always runnable

### <Language> Development Standards

#### Error Handling
<!-- Adapt to your language's idioms -->
- <error handling pattern, e.g., Result types, error returns, exceptions policy>

#### Naming Conventions
<!-- Adapt to your language's conventions -->
- <naming rules for types, functions, variables, constants, etc.>

#### Code Organization
- <organization rules: file structure, module boundaries, etc.>

## Task Management

Tasks are managed in the `management/` folder:

- **`management/backlog.yaml`** - Task index organized by capability
- **`management/tasks/<TASK-ID>.yaml`** - Individual task definitions
- **`management/plans/<TASK-ID>-plan.md`** - Implementation plans
- **`management/plans/<TASK-ID>-verified.md`** - Verification reports

Task workflow: `backlog → planned → in_progress → review → done`

Use slash commands to work with tasks:
- `/plan <TASK-ID>` - Create implementation plan
- `/execute <TASK-ID>` - Implement with TDD
- `/review <TASK-ID>` - Review code quality
- `/improve <TASK-ID>` - Fix issues from review
- `/verify <TASK-ID>` - Verify and complete task

Task workflow: `/plan` → `/execute` → `/review` → `/improve` (if needed) → `/verify`

### Task ID Conventions (Capability-Based)

<!-- Adapt prefixes to your project's capabilities -->

| Prefix | Capability | Description |
|--------|------------|-------------|
| `<PREFIX>` | <capability> | <description> |

### Task Definition Format

Each task in `management/tasks/<TASK-ID>.yaml`:

```yaml
id: <PREFIX>-001
title: "Short descriptive title"
status: backlog
priority: 1
created: YYYY-MM-DD
phase: "M1: <Phase Name>"

dependencies: []

observable: |
  <What you can run to verify the task is complete>

behaviors:
  - "Behavior 1 that can be expressed as a test"
  - "Behavior 2 that can be expressed as a test"

scope: |
  Brief description of what's needed.

definition_of_done:
  - All behavior tests pass
  - Observable output works as specified
  - Test coverage >= 80%
  - No build warnings

complexity: low
estimated_effort: 2-3 hours
```

## Testing Requirements

- **Coverage target**: Near-100% branch coverage on core logic
- **Framework**: <test framework>
- **Test naming**: `MethodName_Scenario_ExpectedResult` (or equivalent convention)

## Git Workflow

### Never Commit to Main

**All development MUST happen on feature branches. No exceptions.**

### Branch Naming
- `feature/<TASK-ID>-description` - New features
- `fix/<TASK-ID>-description` - Bug fixes
- `refactor/<TASK-ID>-description` - Code improvements

### Commit Messages
```
type(scope): brief description

- Detail 1
- Detail 2

Refs: TASK-XXX
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

### Commit Discipline
- **Test commit first**: Commit the failing test before implementation
- **Small commits**: One logical change per commit
- **Green commits**: Every commit should pass all tests

## Build Commands

```bash
# Adapt these to your language/build system
<build command>
<test command>
<test with coverage command>
<format/lint command>
```

## Quality Gates

Before any commit:
- [ ] On feature branch (not main)
- [ ] Build succeeds
- [ ] Tests pass
- [ ] Formatting/linting passes

Before task completion:
- [ ] Coverage >= 80%
- [ ] All Definition of Done items verified
- [ ] CHANGELOG.md updated
````

---

## 4. Management Files

### 4.1 `management/backlog.yaml`

```yaml
schema_version: "2.0"
project: <PROJECT_NAME>

# Completed tasks (moved here for history)
completed: []

# Tasks organized by capability
capabilities:
  <capability_key>:
    description: "<What the user can do>"
    milestone: M1
    tasks:
      - id: <PREFIX>-001
        title: "First task title"
        status: backlog
      # When a task progresses, additional fields are added:
      # - id: <PREFIX>-002
      #   title: "Second task"
      #   status: done
      #   planned_date: 2026-03-07
      #   started_date: 2026-03-07
      #   completed_date: 2026-03-07
      #   verified_by: claude
      #   branch: feature/<PREFIX>-002-description

  <another_capability>:
    description: "<Description>"
    milestone: M2
    tasks: []
```

### 4.2 Task YAML (`management/tasks/<TASK-ID>.yaml`)

```yaml
id: <PREFIX>-001
title: "Concise title derived from vertical slice"
status: backlog
priority: 1
created: 2026-01-01
phase: "M1: <Phase Name>"

dependencies: []

observable: |
  Concrete, runnable verification.
  Use actual HTTP requests/responses, CLI commands, or test execution.

behaviors:
  - "Specific testable behavior 1"
  - "Specific testable behavior 2"
  - "Each maps to one or more tests"

scope: |
  What needs to be built. Reference specific types, interfaces,
  or endpoints. 2-5 sentences.

definition_of_done:
  - "All behavior tests pass"
  - "Observable output works as specified"
  - "Test coverage >= 80%"
  - "No build warnings"

complexity: low  # low | medium | high
estimated_effort: "2-3 hours"
```

---

## 5. Slash Commands

Each file goes in `.claude/commands/<name>.md`. The `$ARGUMENTS` placeholder is replaced with the user's input when the command is invoked.

### 5.1 `/plan` — Plan Task Implementation

**File:** `.claude/commands/plan.md`

**Purpose:** Create a detailed, execution-ready implementation plan for a backlog task. The goal is to make execution mechanical — every decision, edge case, and file change resolved upfront.

**Steps:**
1. **Verify Branch** — Must be on feature branch, not main. Create `feature/<TASK-ID>-description` if needed.
2. **Load Task** — Read task YAML, check backlog status, extract observable/behaviors/scope/DoD/dependencies.
3. **Verify Dependencies** — All dependency tasks must be `done`. Stop if incomplete.
4. **Deep Code Exploration** — Read every file that will be touched. Read every test file. Search for all call sites of changed interfaces. Trace side effects. This is the most critical step.
5. **Reason Through Approach** — Draft approach, challenge it with edge cases, determine execution order, identify every file that will change.
6. **Write Plan** — Create `management/plans/<TASK-ID>-plan.md` with:
   - Exact file paths, before/after code snippets
   - Exact test names with method signatures
   - Test impact summary (which existing tests break and why)
   - Risks and edge cases
   - Steps ordered by blast radius (smallest first)
7. **Commit Plan** — `docs(plan): add implementation plan for <TASK-ID>`
8. **Update Status** — Set backlog status to `planned`, add `planned_date` and `branch`

**Output:** Plan summary, key decisions, implementation step list, open questions.

**Quality Checklist:**
- Every file to be modified has been read (not skimmed)
- Every affected test file has been read
- All call sites of changed interfaces identified
- Before/after code snippets for non-trivial changes
- Impact on existing tests explicitly listed
- Edge cases and risks identified
- Tests specified BEFORE implementation (TDD)

### 5.2 `/execute` — Execute Task Implementation

**File:** `.claude/commands/execute.md`

**Purpose:** Implement a planned task following TDD (Red-Green-Refactor). The plan has done the thinking — execution translates it into code.

**Steps:**
1. **Verify Branch** — Must be on task's feature branch. If no branch exists, stop and run `/plan` first.
2. **Pre-flight Checks** — Load task, verify status is `planned` or `in_progress`, load plan, verify dependencies.
3. **Internalize Plan** — Read plan thoroughly. Read every file in the plan's "Files to modify" tables. Compare current code against plan's snapshots. Reconcile if code has changed.
4. **Update Status** — Set to `in_progress` if currently `planned`.
5. **TDD Implementation Loop** — For each plan step:
   - **RED:** Write failing tests, run to confirm failure for right reason, commit
   - **GREEN:** Implement per plan, re-read own changes, check standards, run tests, commit
   - **REFACTOR:** Check for duplication, responsibility, naming, allocations, data structure fit. Commit if changes made.
6. **Step-level Gate** — After each RED+GREEN+REFACTOR cycle: build, test, format. Review own diff. Cross-reference plan's test impact table.
7. **Track Progress** — Update plan with completed steps and any deviations.
8. **Final Review** — Full diff review, DoD check, observable verification, performance review, standards spot-check.
9. **Completion** — Run full verification suite. Update status to `review`. Push branch.

**When the Plan is Wrong:**
- Small correction → update plan, continue
- Medium correction → update plan, add "Deviations" section, continue
- Large correction → STOP, update plan with findings, suggest re-running `/plan`

### 5.3 `/review` — Code Review

**File:** `.claude/commands/review.md`

**Purpose:** Audit code for standards compliance, bugs, and design issues. Assume the code is wrong until proven otherwise.

**Steps:**
1. **Identify Files** — `git diff --name-only origin/main...HEAD` for source files
2. **Re-read Standards** — Mandatory: read coding standards from CLAUDE.md. Do not work from memory.
3. **Error Handling Audit** — Find every `throw`/error. Classify as expected failure vs truly exceptional. Expected failures must use the project's error return pattern (not exceptions).
4. **Input Validation Audit** — Every public method accepting external input: what happens with null, empty, malformed?
5. **Async/Concurrency Audit** — Check for blocking calls, missing configuration, naming.
6. **Naming Audit** — Every public member checked against naming rules.
7. **Code Organization Audit** — File-per-type, single responsibility, composition, immutable data types.
8. **Correctness Audit** — Edge cases, thread safety, resource management, performance, determinism.
9. **Test Quality Audit** — Error paths covered? Edge cases? Specific assertions? Missing behaviors?
10. **Write Report** — Create `management/reviews/<TASK-ID>-review.md` with findings tables (Critical/High/Medium/Low) and standards compliance summary.

**Verdict:**
- **PASS:** No findings at any severity level
- **FAIL:** Any findings exist → must run `/improve` then `/review` again

### 5.4 `/improve` — Fix Review Findings

**File:** `.claude/commands/improve.md`

**Purpose:** Fix all issues identified by `/review`. Does NOT change task status.

**Steps:**
1. **Verify Branch** — Must be on task's feature branch
2. **Load Context** — Read task, plan, review report. If review PASS → stop, suggest `/verify`. Extract all findings.
3. **Identify Files** — Read every changed source file in full.
4. **Scope Check** — All findings in changed files must be fixed. Only pre-existing issues in unchanged files may be deferred with rationale.
5. **Fix Loop** — For each finding (Critical → High → Medium → Low):
   - Read file before editing
   - If behavior change: TDD (write/update failing test → fix → verify)
   - If structural fix: apply, run tests, commit
   - After each fix: full quality gate (build, test, format)
6. **Generate Report** — Create `management/plans/<TASK-ID>-improved.md` with resolved findings ledger and quality gate results.

**Output:** Count of findings resolved, key fixes, quality gate result, prompt to re-run `/review`.

### 5.5 `/verify` — Verify Task Completion

**File:** `.claude/commands/verify.md`

**Purpose:** Final verification gate. On PASS: creates PR, merges, cleans up. On FAIL: keeps status at `review`.

**Steps:**
1. **Verify Branch** — Must be on task's feature branch, all changes committed and pushed.
2. **Load Context** — Read task, plan, review report, improvement report, backlog status.
3. **Run All Checks** — Build, test, coverage, format. All must pass.
4. **Verify Observable** — Run the task's observable scenario (start API if needed, execute requests, verify responses).
5. **Verify Behaviors** — For each behavior, identify corresponding test(s), verify they exist and pass.
6. **Verify Definition of Done** — Each DoD item verified with evidence.
7. **Verify Plan Completion** — All steps marked complete, all files created/modified as listed.
8. **Code Review** — If review report exists with PASS: spot-check 2-3 items. Otherwise: full manual review against standards.
9. **Verify Commits** — All reference task ID, conventional format, TDD pattern visible.
10. **Generate Report** — Create `management/plans/<TASK-ID>-verified.md`.
11. **Update Changelog** — Add entry under `## [Unreleased]`.

**On PASS:**
- Update status to `done`, add `completed_date` and `verified_by`
- Create PR via `gh pr create`
- Wait for CI, merge (squash), delete branch
- Update local main

**On FAIL:**
- Keep status at `review`
- List issues, suggest `/review` → `/improve` → `/verify`
- Do NOT update changelog or create PR

### 5.6 `/test` — Write Tests

**File:** `.claude/commands/test.md`

**Purpose:** Write comprehensive tests for a component or feature.

**Steps:**
1. Identify test target from scope argument
2. Review testing standards (framework, assertions, mocking, naming)
3. Identify test cases: happy path, edge cases, error cases, state variations
4. Write tests using project conventions
5. Run and verify

### 5.7 `/commit` — Prepare Commit

**File:** `.claude/commands/commit.md`

**Purpose:** Prepare changes for commit following conventional commit standards.

**Steps:**
1. Verify not on main
2. Review changes (`git status`, `git diff`)
3. Pre-commit checks (build, test, format)
4. Stage changes
5. Create commit with conventional message: `type(scope): description`
6. Add task reference in footer: `Refs: TASK-ID`

**TDD Commit Pattern:**
1. `test(scope): add failing tests for feature X` (RED)
2. `feat(scope): implement feature X` (GREEN)
3. `refactor(scope): extract helper for feature X` (REFACTOR)

### 5.8 `/spec` — Specification Lookup

**File:** `.claude/commands/spec.md`

**Purpose:** Look up specification details for a topic. Maps topics to documentation files and extracts requirements, data structures, algorithms, edge cases, constraints.

---

## 6. Skills

Skills differ from commands in that they have a `SKILL.md` file in a subfolder under `.claude/skills/`, and they can have supporting files alongside them.

### 6.1 `/backlog` — Generate Backlog Tasks

**Directory:** `.claude/skills/backlog/`

**Files:**
- `SKILL.md` — Main skill definition
- `task-template.md` — YAML template and guidelines for writing behaviors/observables
- `milestone-mapping.md` — Maps milestones to capability keys, prefixes, phases

**Purpose:** Generate task YAML files for all vertical slices in a milestone from the roadmap.

**Steps:**
1. Validate milestone argument
2. Verify on feature branch
3. Check existing tasks (avoid overwrites)
4. Read roadmap milestone section — extract slices, test scenarios, performance criteria
5. Read specification sections referenced by the milestone
6. Determine cross-milestone dependencies
7. Generate task YAML files (one per vertical slice)
8. Update `management/backlog.yaml`
9. Commit

**Supporting File: `task-template.md`**

Provides:
- Exact YAML schema
- Guidelines for writing good behaviors (4-10 per task, each expressible as a test name)
- Sources for deriving behaviors: interface methods, type definitions, test scenarios, error codes, edge cases
- Observable output examples by task type (data model, storage, REST, parser, integration)
- Complexity/effort guidelines by slice type

**Supporting File: `milestone-mapping.md`**

Provides:
- Table mapping each milestone to capability key, task prefix, phase name
- Shared prefix rules (when milestones share a task prefix, continue numbering)
- Hard dependency chain between milestones
- Expected slice counts per milestone

### 6.2 `/status` — Project Status Dashboard

**Directory:** `.claude/skills/status/`

**Purpose:** Show project progress dashboard.

**Steps:**
1. Load `management/backlog.yaml`, collect all tasks
2. Load task details from YAML files
3. Compute statistics (by status: backlog/planned/in_progress/review/done/blocked)
4. Determine milestone progress (Complete/In Progress/Ready/Not Generated)
5. Find next available tasks (status=backlog, all deps done, in current milestone)
6. Find blocked tasks (deps not done and not in progress)
7. Output dashboard with progress bar, milestone overview table, next tasks, blockers

---

## 7. Task Lifecycle

### State Machine

```
backlog ──/plan──→ planned ──/execute──→ in_progress ──/execute(done)──→ review
                                                                          │
                                              ┌─────────/review(FAIL)─────┤
                                              │                           │
                                              ▼                           │
                                          /improve ──→ /review(PASS) ─────┤
                                                                          │
                                                                   /verify(PASS)
                                                                          │
                                                                          ▼
                                                                        done
```

### Artifacts Produced

| Phase | Command | Artifact |
|-------|---------|----------|
| Planning | `/plan` | `management/plans/<TASK-ID>-plan.md` |
| Execution | `/execute` | Source code, test code |
| Review | `/review` | `management/reviews/<TASK-ID>-review.md` |
| Improvement | `/improve` | `management/plans/<TASK-ID>-improved.md` |
| Verification | `/verify` | `management/plans/<TASK-ID>-verified.md`, PR, CHANGELOG |

### Git Flow per Task

1. Create branch `feature/<TASK-ID>-description` (during `/plan`)
2. Commit plan
3. TDD commits: test → feat → refactor (during `/execute`)
4. Review/improve commits (during `/improve`)
5. Verification report + changelog commit (during `/verify`)
6. Squash merge to main, delete branch (during `/verify`)

---

## 8. Adaptation Guide

When bootstrapping this workflow into a new project, adapt these areas:

### Language-Specific Adaptations

| Area | What to Change |
|------|---------------|
| Build commands | Replace `dotnet build/test/format` with your toolchain |
| Test framework | Replace xUnit/FluentAssertions/Moq with your stack |
| Error handling pattern | Replace `Result<T, TError>` with language idiom |
| Naming conventions | Use language-standard conventions |
| Code organization rules | Adapt to language file/module conventions |
| Async patterns | Adapt to language concurrency model |
| Commit scopes | Replace `core/rdf/sparql/...` with your project's domains |
| Coverage tooling | Replace `XPlat Code Coverage` with your coverage tool |

### Project-Specific Adaptations

| Area | What to Change |
|------|---------------|
| Task ID prefixes | Define based on your project's capabilities |
| Milestone mapping | Map to your project's phases |
| Backlog capabilities | Define based on what users can do with your system |
| Specification references | Point to your docs |
| Performance review checklist | Adapt to your domain's hot paths |
| `.claude/settings.local.json` | Allow your build/test/deploy commands |

### Minimal Setup

For a minimal workflow (no roadmap/backlog generation), you need:

1. `CLAUDE.md` — Project instructions
2. `management/backlog.yaml` — Task index (can be manually populated)
3. `management/tasks/` — Task YAML files (can be manually created)
4. `.claude/commands/plan.md` — Planning
5. `.claude/commands/execute.md` — TDD execution
6. `.claude/commands/review.md` — Code review
7. `.claude/commands/improve.md` — Fix findings
8. `.claude/commands/verify.md` — Verification and merge
9. `.claude/commands/commit.md` — Conventional commits

The `/backlog`, `/status`, `/spec`, and `/test` commands are optional enhancements.

### Settings File

Create `.claude/settings.local.json` with permissions for your build tools:

```json
{
  "permissions": {
    "allow": [
      "Bash(git:*)",
      "Bash(<your-build-tool>:*)",
      "Bash(<your-test-tool>:*)",
      "Bash(gh pr:*)",
      "Bash(gh issue:*)",
      "Bash(ls:*)",
      "Bash(mkdir:*)"
    ]
  }
}
```

---

## Quick Start

To set up this workflow in a new project:

1. **Create the directory structure:**
   ```bash
   mkdir -p .claude/commands .claude/skills/backlog .claude/skills/status
   mkdir -p management/tasks management/plans management/reviews
   mkdir -p docs
   ```

2. **Create `CLAUDE.md`** — Adapt the template from [Section 3](#3-claudemd-template)

3. **Create `management/backlog.yaml`** — Start with the schema from [Section 4.1](#41-managementbacklogyaml)

4. **Create slash commands** — One `.md` file per command in `.claude/commands/`. Use the descriptions in [Section 5](#5-slash-commands) as the basis, adapting language-specific details.

5. **Create skills** (optional) — For `/backlog` and `/status` in `.claude/skills/`

6. **Create `.claude/settings.local.json`** — Allow your build/test commands

7. **Create initial task definitions** — Either manually in `management/tasks/` or via `/backlog`

8. **Start working:** `/plan <first-task-id>`
