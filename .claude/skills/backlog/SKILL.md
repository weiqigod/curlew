---
name: backlog
description: This skill should be used when the user asks to generate task definitions for a milestone (e.g., "/backlog M4", "generate the M4 backlog", "create tasks for milestone N"). Decomposes a milestone from docs/SPECIFICATION.md into vertical slices, writes one YAML task file per slice to management/tasks/, and updates management/backlog.yaml. Uses the Plan agent to do the decomposition.
---

# Backlog Generator

Generate task definitions for milestone: $ARGUMENTS

Workflow position: `/backlog <M>` → (then per task) `/plan` → `/execute` → `/review` → `/improve` (if FAIL) → `/verify`

Not part of `/pipeline`. `/pipeline` operates on one already-generated task at a time.

---

## Step 0: Verify Argument and Prepare Branch

**STOP** if the milestone argument is missing or does not match `^M[1-9][0-9]*$`. Say:
> "Milestone argument required. Example: `/backlog M4`. Received: `<arg>`."

Run now:
```bash
git branch --show-current
git status --porcelain
```

Branch handling:
- If on `main` with a **clean** working tree: create and switch to `chore/backlog-<M>` automatically via `git checkout -b chore/backlog-<M>`, then proceed.
- If on `main` with **uncommitted changes**: **STOP**. Say:
  > "You are on `main` with uncommitted changes. Commit or stash them, then re-run `/backlog <M>`."
- If already on `chore/backlog-<M>`: proceed.
- If on any other branch: **STOP**. Say:
  > "You are on `<branch>`. Switch to `main` (or to `chore/backlog-<M>` if it exists) before running `/backlog <M>`."

---

## Step 1: Validate Milestone

Read `milestone-mapping.md` now. Do NOT work from memory.

Capture from the milestone's section:
- Capability keys (journey stages)
- Task ID prefix (e.g. `M4-`)
- Expected slice count (per capability and total)
- Track notes (if the milestone is heterogeneous — see M4 as the reference)

**STOP** if the milestone has no section in `milestone-mapping.md`. Say:
> "No entry for `<M>` in milestone-mapping.md. Add the milestone section there first, then re-run."

---

## Step 2: Check Milestone Completeness

Read `management/backlog.yaml` now. Do NOT work from memory.

**STOP** if this milestone's tasks already exist AND are all `status: done`. Say:
> "Milestone `<M>` is already complete. Nothing to generate."

**STOP** if the previous milestone's tasks are not all `done` (and the track notes do not allow overlap). Say:
> "Milestone `<prev>` is not complete. Generate only when the prior milestone's tasks are all `done`, to keep dependencies honest."

---

## Step 3: Inventory Existing Tasks

List every `management/tasks/<PREFIX>-*.yaml` and every entry in `management/backlog.yaml` under this milestone's capability keys. Record:
- Highest existing ID number
- A `{id, title}` list to pass to the Plan agent (so it does not duplicate)

**STOP** if the state is inconsistent — task files exist under `management/tasks/` that are not in `backlog.yaml`, or vice versa. Say:
> "Inconsistent state: <N> task files under `management/tasks/` not in `backlog.yaml` (or vice versa). Reconcile by hand before generating more."

---

## Step 4: Read Supporting Files

Read these files now. Do NOT work from memory:
- `task-template.md` — YAML schema, behavior guidelines, complexity criteria, common pitfalls
- The milestone's section of `milestone-mapping.md` — capability keys, DAG, track notes
- `docs/SPECIFICATION.md` — locate the Phase-`<N>` section that maps to `<M>`, and record the line ranges for each feature (do NOT read the whole 9000-line spec)

Emit the located spec line ranges in your notes — the Plan agent will read them directly.

---

## Step 5: Dispatch Plan Agent

**This is the most important step. Do not rush it.**

Use the **Agent tool** with `subagent_type: "Plan"` and `model: "opus"` to decompose the milestone into vertical slices in a dedicated context.

Pass the Plan agent a prompt containing:

1. **Milestone context** — from Step 1: capability keys, task ID prefix, expected slice count, track notes.
2. **Existing tasks** — from Step 3: ID list and title list. The agent MUST NOT duplicate these.
3. **Spec anchors** — from Step 4: the specific `docs/SPECIFICATION.md` line ranges for each capability. Instruct the agent to read those ranges first, widen only as needed. Do NOT ask it to read the whole spec.
4. **Exploration instructions** — the agent must also read `task-template.md`, the relevant section of `milestone-mapping.md`, and `docs/DEVELOPMENT_PHILOSOPHY.md`.
5. **Reasoning instructions** — the agent must:
   - Identify every vertical slice for the milestone (one slice = one task)
   - Draft the full YAML payload per task: `id`, `title`, `behaviors` (4–10, strictly Given/When/Then format), `scope`, `observable` (a concrete runnable command, not "tests pass"), `dependencies`, `complexity`, `estimated_effort`, and `track` (when the milestone has track notes)
   - Build the dependency DAG across all drafted tasks and verify it is acyclic
   - Ensure each `observable` matches the track's observable shape (see Track Notes in `milestone-mapping.md`)
   - Flag any slice where the spec is ambiguous
6. **Output format** — the agent must return:
   - An ordered list of task drafts, each containing all YAML fields from the schema
   - The dependency graph as a summary table
   - Ambiguities and open questions from the spec

**STOP** if the agent returns a slice count outside ±20% of the mapping's expectation. Say:
> "Plan agent returned <N> slices; mapping expected ~<M>. Re-run with a narrower prompt, or adjust the mapping if the count is legitimately different."

Use the Plan agent's output as the sole input for Step 6.

---

## Step 6: Generate Task Files

For each draft returned by the Plan agent, create `management/tasks/<PREFIX>-<NNN>.yaml` following the schema in `task-template.md`. Rules:

- Zero-pad IDs to three digits (`001`, `002`, …)
- Continue numbering from the highest existing ID recorded in Step 3
- Do not deviate from the agent's draft without good reason. If you do, add a `# note:` comment in the YAML explaining why
- Set `status: backlog` and `created: <today YYYY-MM-DD>`

**STOP** before writing any file if the agent's draft lacks any required schema field. Say:
> "Draft for `<PREFIX>-NNN` is missing required field `<field>`. Re-run Step 5 with stricter output instructions."

---

## Step 7: Update backlog.yaml

For each new task, add an entry under the correct capability key in `management/backlog.yaml`:

```yaml
- id: <PREFIX>-NNN
  status: backlog
```

If a capability key does not yet exist, create it with `description:` (from `milestone-mapping.md`) and `milestone: <M>` fields. Add it under the existing milestone sections in the file so the ordering stays readable.

---

## Step 8: Commit

Make exactly two commits on the `chore/backlog-<M>` branch — task files first, then the backlog index.

**Commit A — task files:**
```bash
git add management/tasks/<PREFIX>-*.yaml
git commit -m "$(cat <<'EOF'
chore(task): generate <M> task definitions

- <N> tasks across <K> capabilities
- See management/tasks/<PREFIX>-*.yaml

Refs: <M>
EOF
)"
```

**Commit B — backlog index:**
```bash
git add management/backlog.yaml
git commit -m "$(cat <<'EOF'
chore(task): add <M> tasks to backlog index

Refs: <M>
EOF
)"
```

Two commits — not one — because the task files and the backlog index are logically separate concerns and this matches the per-task conventions used by `/plan` and `/execute`.

---

## Step 9: Output

Emit a summary table (ID, title, complexity, dependency count), total tasks generated, and any ambiguities flagged by the Plan agent.

Close with:
> "Backlog for `<M>` generated. Run `/plan <PREFIX>-001` to start the first task, or `/status` to see the full board."

---

## Quality Checklist

- [ ] Milestone argument validated against `milestone-mapping.md`
- [ ] On a `chore/backlog-<M>` branch, not `main`
- [ ] Previous milestone confirmed complete (or track note allows overlap)
- [ ] Existing tasks inventoried; no duplicate titles
- [ ] `task-template.md`, `milestone-mapping.md`, and spec line ranges all read now
- [ ] Plan agent prompt includes spec anchors (line ranges), not "read the whole spec"
- [ ] Every task has 4–10 Given/When/Then behaviors
- [ ] Every `observable` is a concrete, runnable command matching its track's observable shape
- [ ] Every task has at least one `definition_of_done` item beyond "tests pass"
- [ ] Dependency graph verified acyclic
- [ ] Slice count within ±20% of the mapping's expectation
- [ ] All task files zero-padded to three digits
- [ ] Two commits made: task files first, then backlog index
- [ ] Task file count matches the summary table
