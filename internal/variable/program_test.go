package variable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildProgramHelper(t *testing.T) string {
	t.Helper()
	name := "native helper O'\u00e5"
	goName := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
		goName += ".exe"
	}
	program := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", goName), "build", "-buildvcs=false", "-o", program, "./testdata/programhelper")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build native helper: %v\n%s", err, output)
	}
	return program
}

func helperShellCommand(program, mode string) string {
	quoted := "'" + strings.ReplaceAll(program, "'", "'\\''") + "'"
	if runtime.GOOS == "windows" {
		quoted = "& '" + strings.ReplaceAll(program, "'", "''") + "'"
	}
	return quoted + " " + mode
}

func TestExecuteCommandNativeHelper(t *testing.T) {
	program := buildProgramHelper(t)
	t.Run("output", func(t *testing.T) {
		output, err := ExecuteCommand(context.Background(), helperShellCommand(program, "hello"))
		if err != nil || output != "hello" {
			t.Fatalf("output = %q, error = %v", output, err)
		}
	})
	t.Run("safe_exit_42", func(t *testing.T) {
		output, err := ExecuteCommand(context.Background(), helperShellCommand(program, "exit42"))
		if output != "" || !errors.Is(err, ErrCommandFailed) {
			t.Fatalf("output = %q, error = %v", output, err)
		}
		if !strings.Contains(err.Error(), "code 42") {
			t.Fatalf("missing exit code: %v", err)
		}
		for _, secret := range []string{program, "private-stdout", "private-stderr", "exit42"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error leaks %q: %v", secret, err)
			}
		}
	})
}

func TestExecuteProgram(t *testing.T) {
	program := buildProgramHelper(t)
	t.Run("exact_argv", func(t *testing.T) {
		args := []string{"spaces here", "O'Brien", "\u00e5\u96ea\U0001f642", `a"b`, `C:\trailing\`, "", "%FOO%", "!FOO!", "&", "|", "^", "<", ">", `\"`, "line\nbreak", "--flag=value"}
		assertProgramArgs(t, program, args, nil)
	})
	t.Run("output", func(t *testing.T) {
		for _, output := range []string{"", "  value  ", "a\nb\n\n", "value\r", "a\r\nb\r\n\r\n"} {
			got, err := ExecuteProgram(context.Background(), program, []string{"output", output}, nil)
			want := strings.TrimRight(output, "\n")
			if runtime.GOOS == "windows" && output == "a\r\nb\r\n\r\n" {
				want = "a\r\nb"
			}
			if got != want || err != nil {
				t.Errorf("output %q: got %q, error %v, want %q", output, got, err, want)
			}
		}
	})
	t.Run("child_only_environment", func(t *testing.T) {
		t.Setenv("CURLEW_PROGRAM_INHERITED", "parent")
		t.Setenv("CURLEW_PROGRAM_OVERRIDE", "parent")
		env := []string{"CURLEW_PROGRAM_OVERRIDE=first", "CURLEW_PROGRAM_OVERRIDE=child=value", "CURLEW_PROGRAM_EMPTY=", "CURLEW_PROGRAM_SECRET=private-env"}
		before := append([]string(nil), env...)
		output, err := ExecuteProgram(context.Background(), program, []string{"env"}, env)
		if err != nil {
			t.Fatal(err)
		}
		var actual []string
		if err := json.Unmarshal([]byte(output), &actual); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"CURLEW_PROGRAM_INHERITED=parent", "CURLEW_PROGRAM_OVERRIDE=child=value", "CURLEW_PROGRAM_EMPTY=", "CURLEW_PROGRAM_SECRET=private-env"} {
			count := 0
			for _, entry := range actual {
				if entry == want {
					count++
				}
			}
			if count != 1 {
				t.Errorf("entry %q count = %d, want 1", want, count)
			}
		}
		if os.Getenv("CURLEW_PROGRAM_OVERRIDE") != "parent" || !reflect.DeepEqual(env, before) {
			t.Fatal("parent environment or caller slice mutated")
		}
	})
	t.Run("safe_exit_42_and_diagnostic", func(t *testing.T) {
		output, err := ExecuteProgram(context.Background(), program, []string{"exit42", "private-argv"}, []string{"CURLEW_PROGRAM_SECRET=private-env"})
		assertSafeProgramFailure(t, output, err, program, "private-argv", "private-env", "private-stdout", "private-stderr")
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 42 || !strings.Contains(err.Error(), "code 42") {
			t.Fatalf("missing typed exit 42: %v", err)
		}
		wrapped := fmt.Errorf("provider failed: %w", err)
		if diagnostic := CommandDiagnostic(wrapped); diagnostic != "private-stderr\n" {
			t.Fatalf("diagnostic = %q", diagnostic)
		}
		var failure *commandFailure
		if !errors.As(err, &failure) || failure.stdout != "private-stdout" {
			t.Fatal("stdout not retained privately")
		}
	})
	t.Run("missing", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "private-missing-provider.exe")
		output, err := ExecuteProgram(context.Background(), missing, []string{"private-argv"}, nil)
		assertSafeProgramFailure(t, output, err, missing, "private-missing-provider", "private-argv")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing error cause: %v", err)
		}
	})
	for _, mode := range []string{"invalid-stdout", "invalid-stderr"} {
		t.Run(mode, func(t *testing.T) {
			output, err := ExecuteProgram(context.Background(), program, []string{mode}, nil)
			assertSafeProgramFailure(t, output, err, program)
			if !strings.Contains(err.Error(), "UTF-8") || CommandDiagnostic(err) != "" {
				t.Fatalf("invalid UTF-8 not rejected safely: %v", err)
			}
		})
	}
	t.Run("invalid_input_before_launch", func(t *testing.T) {
		marker := filepath.Join(t.TempDir(), "started")
		for _, test := range []struct {
			program string
			args    []string
			env     []string
		}{
			{"", nil, nil},
			{program + "\x00", nil, nil},
			{program, []string{"touch", marker, "private\x00argument"}, nil},
			{program, []string{"touch", marker, "\xff"}, nil},
			{program, []string{"touch", marker}, []string{"private-env"}},
			{program, []string{"touch", marker}, []string{"=private-env"}},
			{program, []string{"touch", marker}, []string{"PRIVATE=\x00"}},
			{program, []string{"touch", marker}, []string{"PRIVATE=\xff"}},
		} {
			output, err := ExecuteProgram(context.Background(), test.program, test.args, test.env)
			assertSafeProgramFailure(t, output, err, program, "private-env", "private", "PRIVATE")
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid input launched helper: %v", err)
		}
	})
	t.Run("cancelled_before_launch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		marker := filepath.Join(t.TempDir(), "started")
		output, err := ExecuteProgram(ctx, program, []string{"touch", marker}, nil)
		assertSafeProgramFailure(t, output, err, program)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("missing cancellation cause: %v", err)
		}
		if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cancelled call launched helper: %v", err)
		}
	})
	t.Run("cancel_running_native_process", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		marker := filepath.Join(t.TempDir(), "ready")
		result := make(chan error, 1)
		go func() {
			_, err := ExecuteProgram(ctx, program, []string{"wait", marker}, nil)
			result <- err
		}()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		readyTimeout := time.NewTimer(15 * time.Second)
		defer readyTimeout.Stop()
	ready:
		for {
			select {
			case err := <-result:
				t.Fatalf("exited before native ready signal: %v", err)
			case <-readyTimeout.C:
				t.Fatal("native helper did not signal ready")
			case <-ticker.C:
				if _, err := os.Stat(marker); err == nil {
					break ready
				}
			}
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCommandFailed) {
				t.Fatalf("missing cancellation cause: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("native cancellation did not return")
		}
	})
	t.Run("earlier_deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		started := time.Now()
		_, err := ExecuteProgram(ctx, program, []string{"wait"}, nil)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrCommandFailed) || time.Since(started) > 10*time.Second {
			t.Fatalf("earlier deadline not preserved: %v (%v)", err, time.Since(started))
		}
	})
}

func TestExecuteProgramDefaultTimeout(t *testing.T) {
	program := buildProgramHelper(t)
	started := time.Now()
	_, err := ExecuteProgram(context.Background(), program, []string{"wait"}, nil)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrCommandFailed) || elapsed < 29*time.Second || elapsed > 45*time.Second {
		t.Fatalf("default timeout: error = %v, elapsed = %v, want deadline near 30s", err, elapsed)
	}
}

func TestExecuteCommandDefaultTimeout(t *testing.T) {
	program := buildProgramHelper(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	_, err := ExecuteCommand(ctx, helperShellCommand(program, "wait"))
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrCommandFailed) || elapsed < 29*time.Second || elapsed > 45*time.Second || ctx.Err() != nil {
		t.Fatalf("shell timeout: error = %v, elapsed = %v, parent = %v", err, elapsed, ctx.Err())
	}
}

func TestCommandDiagnostic(t *testing.T) {
	for _, err := range []error{nil, errors.New("private-error"), ErrCommandFailed} {
		if got := CommandDiagnostic(err); got != "" {
			t.Errorf("unrelated error diagnostic = %q", got)
		}
	}
}

func assertSafeProgramFailure(t *testing.T, output string, err error, secrets ...string) {
	t.Helper()
	if output != "" || !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("output = %q, error = %v, want ErrCommandFailed and no output", output, err)
	}
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), fmt.Sprintf("%q", secret)) {
			t.Errorf("error leaks %q: %v", secret, err)
		}
	}
}

func assertProgramArgs(t *testing.T, program string, args, env []string) {
	t.Helper()
	before := append([]string(nil), args...)
	output, err := ExecuteProgram(context.Background(), program, append([]string{"argv"}, args...), env)
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	if err := json.Unmarshal([]byte(output), &actual); err != nil {
		t.Fatalf("argv JSON %q: %v", output, err)
	}
	if !reflect.DeepEqual(actual, args) || !reflect.DeepEqual(args, before) {
		t.Fatalf("argv = %#v, want %#v", actual, args)
	}
}
