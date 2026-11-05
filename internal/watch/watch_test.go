package watch

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestDebouncer(t *testing.T) {
	t.Run("single event fires after interval", func(t *testing.T) {
		d := newDebouncer(50 * time.Millisecond)
		defer d.stop()

		d.trigger()

		select {
		case <-d.C:
			// success
		case <-time.After(500 * time.Millisecond):
			t.Fatal("debouncer did not fire")
		}
	})

	t.Run("rapid events coalesce into one fire", func(t *testing.T) {
		d := newDebouncer(50 * time.Millisecond)
		defer d.stop()

		for i := 0; i < 10; i++ {
			d.trigger()
		}

		// Should fire once
		select {
		case <-d.C:
			// success — first fire
		case <-time.After(500 * time.Millisecond):
			t.Fatal("debouncer did not fire")
		}

		// Should not fire again quickly
		select {
		case <-d.C:
			t.Fatal("debouncer fired more than once")
		case <-time.After(100 * time.Millisecond):
			// expected
		}
	})

	t.Run("spaced events fire separately", func(t *testing.T) {
		d := newDebouncer(20 * time.Millisecond)
		defer d.stop()

		d.trigger()
		select {
		case <-d.C:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("first event did not fire")
		}

		d.trigger()
		select {
		case <-d.C:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("second event did not fire")
		}
	})

	t.Run("stop cancels pending fire", func(t *testing.T) {
		d := newDebouncer(100 * time.Millisecond)
		d.trigger()
		d.stop()

		select {
		case <-d.C:
			t.Fatal("debouncer fired after stop")
		case <-time.After(200 * time.Millisecond):
			// expected
		}
	})
}

func TestRun(t *testing.T) {
	t.Run("initial run executes immediately", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		exitCode := Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
		if count.Load() < 1 {
			t.Errorf("run count = %d, want >= 1", count.Load())
		}
	})

	t.Run("rerun on collection file change", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			// Wait for initial run + watcher setup
			time.Sleep(300 * time.Millisecond)
			// Touch the file
			writeFile(t, colPath, minimalCollection+"\n# changed")
			// Wait for debounce + re-run
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() < 2 {
			t.Errorf("run count = %d, want >= 2 (initial + re-run)", count.Load())
		}
	})

	t.Run("rerun on external file change", func(t *testing.T) {
		dir := t.TempDir()
		mkdirAll(t, filepath.Join(dir, "requests"))
		extPath := filepath.Join(dir, "requests", "get.yaml")
		writeFile(t, extPath, `name: External Get
request:
  method: GET
  url: https://example.com`)
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, `name: Test
requests:
  - path: requests/get.yaml`)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			// Modify the external request file
			writeFile(t, extPath, `name: External Get
request:
  method: GET
  url: https://example.com/changed`)
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() < 2 {
			t.Errorf("run count = %d, want >= 2 (initial + re-run on external file change)", count.Load())
		}
	})

	t.Run("rerun on env file change", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)
		mkdirAll(t, filepath.Join(dir, "environments"))
		envPath := filepath.Join(dir, "environments", "dev.yaml")
		writeFile(t, envPath, `variables:
  base_url: http://localhost`)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			// Modify the environment file
			writeFile(t, envPath, `variables:
  base_url: http://localhost:8080`)
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath, "--env", "dev"},
			EnvName:        "dev",
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() < 2 {
			t.Errorf("run count = %d, want >= 2 (initial + re-run on env file change)", count.Load())
		}
	})

	t.Run("rerun on dotenv change", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)
		dotenvPath := filepath.Join(dir, ".env")
		writeFile(t, dotenvPath, "API_KEY=secret")

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			// Modify the .env file
			writeFile(t, dotenvPath, "API_KEY=newsecret")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() < 2 {
			t.Errorf("run count = %d, want >= 2 (initial + re-run on dotenv change)", count.Load())
		}
	})

	t.Run("ignores unrelated file changes", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			// Write an unrelated file in the same dir
			writeFile(t, filepath.Join(dir, "unrelated.txt"), "hello")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() != 1 {
			t.Errorf("run count = %d, want 1 (initial only)", count.Load())
		}
	})

	t.Run("clean shutdown on context cancel", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		exitCode := Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0", exitCode)
		}
	})

	t.Run("preserves args across reruns", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var mu sync.Mutex
		var capturedArgs [][]string
		ctx, cancel := context.WithCancel(context.Background())

		wantArgs := []string{colPath, "--var", "key=val"}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           wantArgs,
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc: func(args []string, _, _ io.Writer) RunResult {
				mu.Lock()
				capturedArgs = append(capturedArgs, args)
				mu.Unlock()
				return RunResult{}
			},
		})

		mu.Lock()
		defer mu.Unlock()
		for i, args := range capturedArgs {
			if len(args) != len(wantArgs) {
				t.Errorf("run %d: args length = %d, want %d; got %v", i, len(args), len(wantArgs), args)
				continue
			}
			for j := range args {
				if args[j] != wantArgs[j] {
					t.Errorf("run %d: args[%d] = %q, want %q", i, j, args[j], wantArgs[j])
				}
			}
		}
	})

	t.Run("reruns after rapid saves", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var count atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			// Rapid successive saves
			for i := 0; i < 5; i++ {
				writeFile(t, colPath, minimalCollection+"\n# "+string(rune('a'+i)))
				time.Sleep(10 * time.Millisecond)
			}
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       100 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { count.Add(1); return RunResult{} },
		})

		if count.Load() < 2 {
			t.Errorf("run count = %d, want at least 2 (initial and a file-change rerun)", count.Load())
		}
	})

	t.Run("handles RunFunc error gracefully", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		stderr := &bytes.Buffer{}
		exitCode := Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         stderr,
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{ExitCode: 1} },
		})

		// Watch should continue despite RunFunc returning non-zero
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0 (watch continues)", exitCode)
		}
	})

	t.Run("running totals accumulate across reruns", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var callCount atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			RunFunc: func(_ []string, _, _ io.Writer) RunResult {
				callCount.Add(1)
				return RunResult{Passed: 2, Failed: 1, Total: 3}
			},
		})

		out := stdout.String()
		// After 2 runs, should show "Totals (2 runs): 4 passed, 2 failed"
		if !strings.Contains(out, "Totals (2 runs)") {
			t.Errorf("stdout = %q, want to contain 'Totals (2 runs)'", out)
		}
	})

	t.Run("running totals include initial run", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			RunFunc: func(_ []string, _, _ io.Writer) RunResult {
				return RunResult{Passed: 3, Total: 3}
			},
		})

		out := stdout.String()
		if !strings.Contains(out, "Totals (1 run)") {
			t.Errorf("stdout = %q, want to contain 'Totals (1 run)'", out)
		}
	})

	t.Run("parse error after edit shows error and continues watching", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		var callCount atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			// Wait for initial run + watcher setup
			time.Sleep(300 * time.Millisecond)
			// First edit — simulate parse error (RunFunc returns non-zero)
			writeFile(t, colPath, "invalid: [yaml")
			time.Sleep(300 * time.Millisecond)
			// Second edit — back to valid
			writeFile(t, colPath, minimalCollection+"\n# fixed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         &bytes.Buffer{},
			Stderr:         &bytes.Buffer{},
			RunFunc: func(_ []string, _, _ io.Writer) RunResult {
				n := callCount.Add(1)
				if n == 2 {
					// Simulate parse error on second run
					return RunResult{ExitCode: 3}
				}
				return RunResult{Passed: 1, Total: 1}
			},
		})

		// Should have at least 3 runs: initial + error + recovery
		if callCount.Load() < 3 {
			t.Errorf("run count = %d, want >= 3 (initial + error + recovery)", callCount.Load())
		}
	})

	t.Run("clear flag emits ANSI clear before rerun", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			ClearScreen:    true,
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if !strings.Contains(out, "\033[2J\033[H") {
			t.Errorf("stdout = %q, want to contain ANSI clear sequence", out)
		}
	})

	t.Run("clear flag does not clear on initial run", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			ClearScreen:    true,
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if strings.Contains(out, "\033[2J") {
			t.Errorf("initial run should not clear screen, got %q", out)
		}
	})

	t.Run("json format suppresses separator", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			Format:         "json",
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if strings.Contains(out, "Re-running") {
			t.Errorf("json mode should suppress separator, got %q", out)
		}
	})

	t.Run("json format suppresses watching message", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			Format:         "json",
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if strings.Contains(out, "Watching for changes...") {
			t.Errorf("json mode should suppress watching message, got %q", out)
		}
	})

	t.Run("json format suppresses running totals", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			Format:         "json",
			RunFunc: func(_ []string, _, _ io.Writer) RunResult {
				return RunResult{Passed: 2, Total: 2}
			},
		})

		out := stdout.String()
		if strings.Contains(out, "Totals") {
			t.Errorf("json mode should suppress totals, got %q", out)
		}
	})

	t.Run("output shows watching message after initial run", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if !strings.Contains(out, "Watching for changes...") {
			t.Errorf("stdout = %q, want to contain 'Watching for changes...'", out)
		}
	})

	t.Run("watching message lists watched files", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(200 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       10 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		out := stdout.String()
		if !strings.Contains(out, "col.yaml") {
			t.Errorf("stdout = %q, want to contain watched file name 'col.yaml'", out)
		}
	})

	t.Run("output includes rerun separator", func(t *testing.T) {
		dir := t.TempDir()
		colPath := filepath.Join(dir, "col.yaml")
		writeFile(t, colPath, minimalCollection)

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &bytes.Buffer{}

		go func() {
			time.Sleep(300 * time.Millisecond)
			writeFile(t, colPath, minimalCollection+"\n# changed")
			time.Sleep(300 * time.Millisecond)
			cancel()
		}()

		Run(ctx, Config{
			CollectionPath: colPath,
			Args:           []string{colPath},
			Debounce:       50 * time.Millisecond,
			Stdout:         stdout,
			Stderr:         &bytes.Buffer{},
			RunFunc:        func(_ []string, _, _ io.Writer) RunResult { return RunResult{} },
		})

		output := stdout.String()
		if !strings.Contains(output, "Re-running") {
			t.Errorf("stdout = %q, want to contain 'Re-running'", output)
		}
		if !strings.Contains(output, "changed: col.yaml") {
			t.Errorf("stdout = %q, want to contain 'changed: col.yaml'", output)
		}
	})
}

func TestRunningTotals(t *testing.T) {
	tests := []struct {
		name    string
		results []RunResult
		want    RunningTotals
	}{
		{"zero value is valid", nil, RunningTotals{}},
		{
			"single add",
			[]RunResult{{Passed: 3, Failed: 1, Total: 4}},
			RunningTotals{Runs: 1, Passed: 3, Failed: 1, Total: 4},
		},
		{"multiple adds accumulate", []RunResult{
			{Passed: 2, Failed: 1, Total: 3},
			{Passed: 3, Failed: 0, Total: 3},
		}, RunningTotals{Runs: 2, Passed: 5, Failed: 1, Total: 6}},
		{"skipped counts accumulate", []RunResult{
			{Passed: 1, Skipped: 2, Total: 3},
			{Passed: 2, Skipped: 1, Total: 3},
		}, RunningTotals{Runs: 2, Passed: 3, Skipped: 3, Total: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var totals RunningTotals
			for _, r := range tt.results {
				totals.Add(r)
			}
			if totals != tt.want {
				t.Errorf("got %+v, want %+v", totals, tt.want)
			}
		})
	}
}

func TestPrintRunningTotals(t *testing.T) {
	tests := []struct {
		name     string
		useColor bool
		totals   RunningTotals
		want     string
	}{
		{
			"basic output", false,
			RunningTotals{Runs: 2, Passed: 5, Failed: 1, Skipped: 0},
			"Totals (2 runs): 5 passed, 1 failed, 0 skipped",
		},
		{
			"singular run", false,
			RunningTotals{Runs: 1, Passed: 3, Failed: 0, Skipped: 0},
			"Totals (1 run): 3 passed, 0 failed, 0 skipped",
		},
		{
			"with color", true,
			RunningTotals{Runs: 1, Passed: 3, Failed: 0},
			"\033[90m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printRunningTotals(&buf, tt.useColor, &tt.totals)
			out := buf.String()
			if !strings.Contains(out, tt.want) {
				t.Errorf("output = %q, want to contain %q", out, tt.want)
			}
		})
	}
}

func TestPrintWatchStatus(t *testing.T) {
	tests := []struct {
		name     string
		useColor bool
		wantMsg  string
		wantFile string
	}{
		{"basic output", false, "Watching for changes...", "col.yaml"},
		{"with color", true, "\033[36mWatching for changes...\033[0m", "col.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			colPath := filepath.Join(dir, "col.yaml")
			writeFile(t, colPath, minimalCollection)

			wp, err := CollectPaths(colPath, "")
			if err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			printWatchStatus(&buf, tt.useColor, wp)
			out := buf.String()
			if !strings.Contains(out, tt.wantMsg) {
				t.Errorf("output = %q, want to contain %q", out, tt.wantMsg)
			}
			if !strings.Contains(out, tt.wantFile) {
				t.Errorf("output = %q, want to contain %q", out, tt.wantFile)
			}
		})
	}
}

func TestSyncWatchDirs(t *testing.T) {
	t.Run("adds new and removes stale directories", func(t *testing.T) {
		dir1 := t.TempDir()
		dir2 := t.TempDir()
		dir3 := t.TempDir()

		w, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w.Close() }()

		_ = w.Add(dir1)
		_ = w.Add(dir2)
		oldDirs := []string{dir1, dir2}
		newDirs := []string{dir2, dir3}

		syncWatchDirs(w, oldDirs, newDirs, io.Discard)

		watchList := w.WatchList()
		watchSet := make(map[string]struct{}, len(watchList))
		for _, p := range watchList {
			watchSet[p] = struct{}{}
		}

		if _, ok := watchSet[dir1]; ok {
			t.Errorf("dir1 should have been removed but is still watched")
		}
		if _, ok := watchSet[dir2]; !ok {
			t.Errorf("dir2 should still be watched")
		}
		if _, ok := watchSet[dir3]; !ok {
			t.Errorf("dir3 should have been added")
		}
	})

	t.Run("no-op when dirs unchanged", func(t *testing.T) {
		dir1 := t.TempDir()
		w, err := fsnotify.NewWatcher()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = w.Close() }()

		_ = w.Add(dir1)
		dirs := []string{dir1}

		stderr := &bytes.Buffer{}
		syncWatchDirs(w, dirs, dirs, stderr)

		if stderr.Len() != 0 {
			t.Errorf("unexpected stderr output: %s", stderr.String())
		}
		if len(w.WatchList()) != 1 {
			t.Errorf("watch list length = %d, want 1", len(w.WatchList()))
		}
	})
}

// TestRun_PassesConfigWritersToRunFunc verifies that Run passes the Config's
// Stdout and Stderr writers through to the RunFunc call (M7-005).
func TestRun_PassesConfigWritersToRunFunc(t *testing.T) {
	dir := t.TempDir()
	colPath := filepath.Join(dir, "x.yaml")
	writeFile(t, colPath, minimalCollection)

	cfgStdout := &bytes.Buffer{}
	cfgStderr := &bytes.Buffer{}

	var (
		capturedStdout io.Writer
		capturedStderr io.Writer
		mu             sync.Mutex
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = Run(ctx, Config{
		CollectionPath: colPath,
		Args:           []string{colPath},
		Stdout:         cfgStdout,
		Stderr:         cfgStderr,
		Debounce:       10 * time.Millisecond,
		RunFunc: func(_ []string, stdout, stderr io.Writer) RunResult {
			mu.Lock()
			capturedStdout = stdout
			capturedStderr = stderr
			mu.Unlock()
			return RunResult{}
		},
	})

	mu.Lock()
	defer mu.Unlock()
	if capturedStdout != cfgStdout {
		t.Errorf("RunFunc received stdout %p; want cfg.Stdout %p", capturedStdout, cfgStdout)
	}
	if capturedStderr != cfgStderr {
		t.Errorf("RunFunc received stderr %p; want cfg.Stderr %p", capturedStderr, cfgStderr)
	}
}

const minimalCollection = `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`
