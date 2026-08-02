---
name: status
description: This skill should be used when the user asks for project status, progress, or a task dashboard (e.g., "/status", "show status", "what's the progress", "which tasks are done", "what's next", "what's blocked"). Reads management/backlog.yaml and management/tasks/*.yaml, computes milestone progress, and prints a dashboard with current, next-up, and blocked tasks.
---

# Project Status Dashboard

Show the current project status.

## Instructions

1. **Load Backlog** — Read `management/backlog.yaml`. Collect all tasks across all journey stages.

2. **Load Task Details** — For each task with a YAML file in `management/tasks/`, read it to get full status, priority, and dependencies.

3. **Compute Statistics** — Count tasks by status:
   - `backlog` — Not yet planned
   - `planned` — Plan created, ready for execution
   - `in_progress` — Currently being implemented
   - `review` — Implementation done, under review
   - `done` — Verified and merged
   - `blocked` — Dependencies not met

4. **Milestone Progress** — For each milestone:
   - **Complete** — All tasks done
   - **In Progress** — Some tasks done or in progress
   - **Ready** — Tasks generated, none started
   - **Not Generated** — No tasks exist yet

5. **Next Available Tasks** — Find tasks where:
   - Status is `backlog`
   - All dependencies are `done`
   - In the current (earliest incomplete) milestone

6. **Blocked Tasks** — Find tasks where dependencies are not `done` and not `in_progress`.

7. **Output Dashboard**:

```
## Project Status: ApiTool

Progress: [████████░░░░░░░░░░░░] 40% (12/30 tasks done)

### Milestones

| Milestone | Status | Done | Total | Progress |
|-----------|--------|------|-------|----------|
| M1: Core CLI | In Progress | 12 | 25 | 48% |
| M2: Advanced | Not Generated | 0 | 0 | — |

### Current Tasks

| ID | Title | Status | Branch |
|----|-------|--------|--------|
| M1-002 | All HTTP methods, headers, query params, JSON body | in_progress | feature/M1-002-http-methods |

### Next Up

| ID | Title | Complexity | Dependencies Met |
|----|-------|-----------|-----------------|
| M1-004 | Assert on status code | medium | ✓ |

### Blocked

| ID | Title | Waiting On |
|----|-------|-----------|
| M1-019 | Terminal output with colors | M1-006 |
```
