package variable

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsProgramEnvironment(t *testing.T) {
	program := buildProgramHelper(t)
	t.Setenv("Curlew_Mixed_Environment", "parent")
	output, err := ExecuteProgram(context.Background(), program, []string{"env"}, []string{"CURLEW_MIXED_ENVIRONMENT=first", "curlew_mixed_environment=last"})
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		name, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "curlew_mixed_environment") {
			count++
			if value != "last" {
				t.Errorf("override = %q, want last", value)
			}
		}
	}
	if count != 1 || os.Getenv("Curlew_Mixed_Environment") != "parent" {
		t.Fatalf("count = %d or parent environment changed", count)
	}
}

func TestWindowsProgramBatch(t *testing.T) {
	program := buildProgramHelper(t)
	t.Setenv("CURLEW_TEST_NATIVE", program)
	for _, extension := range []string{".cmd", ".bat", ".CMD"} {
		t.Run(extension, func(t *testing.T) {
			wrapper := filepath.Join(t.TempDir(), "forward O'\u00e5 %FOO% !FOO! &^()"+extension)
			if err := os.WriteFile(wrapper, []byte("@\"%CURLEW_TEST_NATIVE%\" %*\r\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Run("exact_argv", func(t *testing.T) {
				args := []string{"spaces here", "O'Brien", "\u00e5\u96ea\U0001f642", `C:\trailing\`, "", "%FOO%", "!FOO!", "&", "|", "^", "<", ">", "a&b|c^d<e>f", "%PATH%", "%1", "%*", "%%", "()", "  spaced  ", "=", "/?", `\\`}
				assertProgramArgs(t, wrapper, args, []string{"FOO=must-not-expand"})
			})
			t.Run("safe_exit_42", func(t *testing.T) {
				output, err := ExecuteProgram(context.Background(), wrapper, []string{"exit42"}, nil)
				assertSafeProgramFailure(t, output, err, wrapper, "private-stderr")
				if !strings.Contains(err.Error(), "code 42") || CommandDiagnostic(err) != "private-stderr\n" {
					t.Fatalf("wrapper failure lost exit or stderr: %v, diagnostic %q", err, CommandDiagnostic(err))
				}
			})
			t.Run("unsupported_before_launch", func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "started")
				for _, arg := range []string{`double"quote`, `"& echo private-injection`, "line\nbreak", "line\rbreak", "nul\x00byte"} {
					output, err := ExecuteProgram(context.Background(), wrapper, []string{"touch", marker, arg}, nil)
					assertSafeProgramFailure(t, output, err, wrapper, "private-injection", arg)
					if !strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "invalid") {
						t.Errorf("rejection not explicit: %v", err)
					}
				}
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unsupported argument launched wrapper: %v", err)
				}
			})
		})
	}
}