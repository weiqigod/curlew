# Implementation Plan: M2-011

## Overview

Add a `watch` command that monitors collection files and related resources (external requests, environments, .env, curlew.yaml) for changes, automatically re-running the collection with debouncing on file save.

## Task Details
- **ID:** M2-011
- **Title:** Watch mode with file system monitoring
- **Phase:** M2: Watch Mode
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | CLI, parsing, HTTP, basic output | done |
| M1-019 | Terminal colors and formatting | done |

## Implementation Steps

### Step 1: Add `ExternalFiles` Field to `parser.Collection`

**Rationale:** Smallest blast radius — purely additive. The watch package needs to know which external request files a collection references. Currently `resolveExternalReferences` resolves paths internally but discards the path information after merging the request content. Adding this field is a prerequisite for path collection.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `ExternalFiles []string` field to `Collection` |
| `internal/parser/external.go` | modify | Return resolved file paths from `resolveExternalReferences` |
| `internal/parser/parser.go` | modify | Wire returned paths into `Collection.ExternalFiles` |
| `internal/parser/parser_test.go` | modify | Add tests for `ExternalFiles` population |

#### Current Code

`internal/parser/collection.go` (lines 118-126):
```go
type Collection struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Variables   SensitiveVars `yaml:"variables,omitempty"`
	Setup       []RequestItem `yaml:"setup,omitempty"`
	Requests    []RequestItem `yaml:"requests"`
	Teardown    []RequestItem `yaml:"teardown,omitempty"`
	Options     Options       `yaml:"options,omitempty"`
}
```

#### New Code

```go
type Collection struct {
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Variables     SensitiveVars `yaml:"variables,omitempty"`
	Setup         []RequestItem `yaml:"setup,omitempty"`
	Requests      []RequestItem `yaml:"requests"`
	Teardown      []RequestItem `yaml:"teardown,omitempty"`
	Options       Options       `yaml:"options,omitempty"`
	ExternalFiles []string      `yaml:"-"` // resolved external file paths, populated by ParseFile
}
```

`internal/parser/external.go` — `resolveExternalReferences` signature change:

```go
// Before:
func resolveExternalReferences(collectionPath string, items []RequestItem, visited map[string]bool) ([]RequestItem, error) {

// After:
func resolveExternalReferences(collectionPath string, items []RequestItem, visited map[string]bool) ([]RequestItem, []string, error) {
```

The function accumulates resolved `extPath` values into a `[]string` and returns them.

`internal/parser/parser.go` — wire up in `ParseFile`:

```go
// Before (line 62-75):
for _, section := range []*[]RequestItem{&col.Setup, &col.Requests, &col.Teardown} {
    if len(*section) == 0 {
        continue
    }
    *section, err = resolveExternalReferences(path, *section, visited)
    if err != nil { ... }
}

// After:
for _, section := range []*[]RequestItem{&col.Setup, &col.Requests, &col.Teardown} {
    if len(*section) == 0 {
        continue
    }
    var extFiles []string
    *section, extFiles, err = resolveExternalReferences(path, *section, visited)
    if err != nil { ... }
    col.ExternalFiles = append(col.ExternalFiles, extFiles...)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_ExternalFiles(t *testing.T) {
    tests := []struct {
        name          string
        setupFiles    map[string]string // relative path -> content
        wantExtCount  int
        wantExtNames  []string // basenames of expected external files
    }{
        {"no external refs", ...},
        {"single external ref in requests", ...},
        {"multiple external refs across sections", ...},
        {"setup and teardown external refs", ...},
    }
}
```

#### Impact on Existing Tests
- `internal/parser/external_test.go` — tests that call `resolveExternalReferences` directly will need to accept the additional `[]string` return value. No assertion changes needed, just destructuring the extra return.
- All other parser tests — unaffected (Collection struct gains an additive field).

---

### Step 2: Add `fsnotify` Dependency

**Rationale:** Must happen before any watch code can reference fsnotify types. Trivial change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `go.mod` | modify | Add `github.com/fsnotify/fsnotify` dependency |
| `go.sum` | auto | Updated by `go get` |

#### Command

```bash
go get github.com/fsnotify/fsnotify
```

---

### Step 3: Create `internal/watch/paths.go` — File Path Collection

**Rationale:** Before building the watcher, we need pure-logic path collection — what files should be watched for a given collection run. This is testable in isolation without fsnotify.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/paths.go` | create | `WatchPaths` type and `CollectPaths` function |
| `internal/watch/paths_test.go` | create | Table-driven tests for path collection |

#### New Code

```go
package watch

import (
	"path/filepath"

	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/parser"
)

// WatchPaths holds all file paths that should be monitored for a collection run.
type WatchPaths struct {
	Collection    string   // absolute path to the main collection file
	ExternalFiles []string // absolute paths to external request files
	EnvFile       string   // absolute path to environment file (empty if not used)
	DotEnv        string   // absolute path to .env file (empty if not found)
	ProjectConfig string   // absolute path to curlew.yaml (empty if not found)
}

// All returns a deduplicated, sorted list of all non-empty file paths to watch.
func (wp *WatchPaths) All() []string

// Dirs returns a deduplicated, sorted list of directories containing watched files.
func (wp *WatchPaths) Dirs() []string

// CollectPaths determines all files related to a collection run.
// Uses parser.ParseFile to discover external file references and
// config package to locate environment/project/dotenv files.
func CollectPaths(collectionPath string, envName string) (*WatchPaths, error)
```

`CollectPaths` implementation:
1. `filepath.Abs(collectionPath)` → `wp.Collection`
2. `parser.ParseFile(collectionPath)` → `col.ExternalFiles` → `wp.ExternalFiles`
3. If `envName != ""`: `config.FindEnvironmentFile(envName, collectionDir)` → `wp.EnvFile`
4. `config.FindProjectRoot(collectionDir)` → if found, project config path → `wp.ProjectConfig`
5. Check for `.env` in project root (or collection dir) → `wp.DotEnv`

#### Tests to Write FIRST (RED phase)

```go
func TestCollectPaths(t *testing.T) {
    tests := []struct {
        name        string
        // setup: temp dir structure
        envName     string
        wantErr     bool
        wantPaths   func(tmpDir string) WatchPaths
    }{
        {"collection only - no extras"},
        {"with environment file"},
        {"with dotenv file"},
        {"with project config"},
        {"with external request refs"},
        {"all sources present"},
        {"missing collection file - error"},
        {"missing env file - error when specified"},
        {"dotenv missing is not an error"},
    }
}

func TestWatchPaths_All(t *testing.T) {
    tests := []struct {
        name  string
        paths WatchPaths
        want  []string
    }{
        {"deduplicates same path"},
        {"excludes empty strings"},
        {"sorted output"},
    }
}

func TestWatchPaths_Dirs(t *testing.T) {
    tests := []struct {
        name  string
        paths WatchPaths
        want  []string
    }{
        {"unique directories"},
        {"single directory"},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — new package.

---

### Step 4: Create `internal/watch/watch.go` — Core Watcher

**Rationale:** The main watch loop with debouncing, fsnotify integration, and re-run logic. Depends on Step 3 for path collection.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | create | `Config`, `Run`, debouncer |
| `internal/watch/watch_test.go` | create | Unit tests for debouncer and integration tests for Run |

#### New Code

```go
package watch

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config holds the configuration for a watch session.
type Config struct {
	CollectionPath string         // path to collection file
	Args           []string       // full CLI args to pass to RunFunc on each re-run
	EnvName        string         // --env flag value (for path collection)
	Debounce       time.Duration  // debounce interval (default 500ms if zero)
	Stdout         io.Writer      // watch status messages
	Stderr         io.Writer      // error output
	UseColor       bool           // ANSI color codes
	RunFunc        func([]string) int // function to call for each run
}

// Run starts the file watch loop. Blocks until ctx is cancelled.
// Performs an initial run, then re-runs whenever watched files change.
// Returns 0 on clean shutdown.
func Run(ctx context.Context, cfg Config) int

// debouncer coalesces rapid file change events.
type debouncer struct {
	interval time.Duration
	timer    *time.Timer
	C        chan struct{}
}

func newDebouncer(interval time.Duration) *debouncer
func (d *debouncer) trigger()
func (d *debouncer) stop()
```

**Watch loop design:**

```
1. Initial run: call RunFunc(Args)
2. CollectPaths → get all files to watch
3. Create fsnotify.Watcher, add Dirs()
4. Loop:
   a. Select on ctx.Done(), watcher.Events, watcher.Errors
   b. On event: check if event.Name matches a watched file
   c. If match: trigger debouncer
   d. On debounce fire:
      - Print separator with timestamp and changed file
      - Call RunFunc(Args)
      - Re-collect paths (files may have changed)
      - Update fsnotify watches
5. On ctx.Done(): close watcher, return 0
```

**Terminal output between re-runs:**
```
--- Re-running (changed: tests.yaml) at 14:32:05 ---
```
With color: separator in cyan.

#### Tests to Write FIRST (RED phase)

```go
func TestDebouncer(t *testing.T) {
    tests := []struct {
        name     string
        // scenario described in test body
    }{
        {"single event fires after interval"},
        {"rapid events coalesce into one fire"},
        {"spaced events fire separately"},
        {"stop cancels pending fire"},
    }
}

func TestRun(t *testing.T) {
    tests := []struct {
        name string
    }{
        {"initial run executes immediately"},
        {"rerun on collection file change"},
        {"rerun on external file change"},
        {"rerun on env file change"},
        {"rerun on dotenv change"},
        {"ignores unrelated file changes"},
        {"clean shutdown on context cancel"},
        {"preserves args across reruns"},
        {"debounces rapid saves"},
        {"handles parse error gracefully"},
        {"refreshes watch list after rerun"},
    }
}
```

**Testing approach:** The `RunFunc` field in `Config` enables test injection — tests provide a mock function that records call count and arguments. For file change tests, use `t.TempDir()` with real files, write changes, and verify the mock was called. Use short debounce intervals (10ms) in tests for speed.

#### Impact on Existing Tests
- No existing tests affected — new package.

---

### Step 5: Wire Up `watchCmd` in `cmd/curlew/main.go`

**Rationale:** Integration point where watch becomes a user-visible command. Must come after the watch package is built (Steps 3-4).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `case "watch":`, `watchCmd` function, update `printHelp` |
| `cmd/curlew/main_test.go` | modify | Add integration tests for watch command |

#### New Code

In `run()` switch statement:
```go
case "watch":
    return watchCmd(args[1:])
```

New function:
```go
func watchCmd(args []string) int {
    file, envName, _, _, _, _, noColor, _, _, err := parseRunArgs(args)
    if err != nil {
        _, _ = fmt.Fprintln(os.Stderr, "Usage: curlew watch <collection-file> [--env <name>] [--var key=value ...] [--no-color] [-v] [-vv] [-q]")
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        errOut.StructuredError(err)
        return 1
    }

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    useColor := shouldUseColor(os.Stdout, noColor)

    return watch.Run(ctx, watch.Config{
        CollectionPath: file,
        Args:           args,
        EnvName:        envName,
        Debounce:       500 * time.Millisecond,
        Stdout:         os.Stdout,
        Stderr:         os.Stderr,
        UseColor:       useColor,
        RunFunc:        runCmd,
    })
}
```

Help text addition:
```go
fmt.Println("  watch <file>    Watch collection and re-run on file changes")
```

Watch Options section:
```go
fmt.Println()
fmt.Println("Watch Options:")
fmt.Println("  Accepts all Run Options, plus:")
fmt.Println("  Ctrl+C          Stop watching and exit")
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_watch_command_recognized(t *testing.T) {
    // verify "watch" is not "unknown command"
}

func TestWatchCmd_missing_file(t *testing.T) {
    // verify usage error on no args
}

// Integration test with built binary (optional, may be deferred to smoke test)
```

#### Impact on Existing Tests
- `TestRun_unknown_command` — unaffected (uses "bogus")
- Help text tests (if any exact-match assertions) — may need update for new "watch" line

---

### Step 6: Update Smoke Tests

**Rationale:** Observable verification. The smoke test validates the watch command works end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add watch mode smoke scenario |

#### Smoke Test Approach

```bash
# Watch mode: start watcher, modify file, verify re-run, send SIGINT
curlew watch smoke/tests.yaml &
WATCH_PID=$!
sleep 1
touch smoke/tests.yaml  # trigger a re-run
sleep 1
kill -INT $WATCH_PID
wait $WATCH_PID
# Check exit code is 0
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/external_test.go` | Tests calling `resolveExternalReferences` | signature change | add `_` for extra return value |
| `internal/parser/parser_test.go` | existing ParseFile tests | none | no changes needed |
| `cmd/curlew/main_test.go` | help/unknown command tests | minor | verify no breakage, add watch tests |
| All other test files | — | none | — |

## Risks and Edge Cases

- **Risk:** Watched file deleted during watch session
  **Mitigation:** fsnotify emits `Remove` event. Print warning, continue watching remaining files. On next re-parse, parser reports the missing file error which is displayed. User can recreate the file.

- **Risk:** New external file added to collection during watch
  **Mitigation:** After each re-run, re-call `CollectPaths` to refresh the watch list. New files are added to fsnotify; removed references are unwatched.

- **Risk:** YAML syntax error on re-parse
  **Mitigation:** `runCmd` already handles parse errors (exit code 3, error output). The watcher displays the error and continues watching — user fixes and saves again.

- **Risk:** Editor write patterns (write-to-temp-then-rename)
  **Mitigation:** 500ms debounce handles this. Watch directories (not individual files) to catch rename events. fsnotify on macOS/kqueue handles renames.

- **Risk:** fsnotify platform differences
  **Mitigation:** fsnotify abstracts OS differences. Watching directories (via `Dirs()`) is more reliable across platforms than watching individual files.

- **Risk:** `resolveExternalReferences` signature change breaks external_test.go
  **Mitigation:** Unexported function, only called within parser package. Update call sites and test destructuring — a 2-minute fix.

- **Edge case:** Watch from different directory than collection
  **Mitigation:** All path resolution uses `filepath.Abs` relative to collection directory, matching `runCmd` behavior.

- **Edge case:** Debounce timer race conditions
  **Mitigation:** Channel-based debouncer design avoids shared state. Timer reset is done in the same goroutine that reads events.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Terminal 1:
curlew watch tests.yaml

# Terminal 2:
# Edit tests.yaml and save

# Observe: collection re-runs in Terminal 1
# Press Ctrl+C in Terminal 1 — clean exit

go test ./internal/watch/...
```
