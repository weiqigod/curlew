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
	t.Chdir(t.TempDir())
	t.Setenv("CURLEW_TEST_NATIVE", program)
	for _, extension := range []string{".cmd", ".bat", ".CMD"} {
		t.Run(extension, func(t *testing.T) {
			wrapper := filepath.Join(t.TempDir(), "forward O'\u00e5 %FOO% !FOO! &^()"+extension)
			if err := os.WriteFile(wrapper, []byte("@\"%CURLEW_TEST_NATIVE%\" %*\r\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Run("exact_argv", func(t *testing.T) {
				args := []string{"spaces here", "O'Brien", "\u00e5\u96ea\U0001f642", `C:\trailing\`, "", "%FOO%", "!FOO!", "&", "|", "^", "<", ">", "a&b|c^d<e>f", "%PATH%", "%1", "%*", "%%", "()", "  spaced  ", "=", "/?", `\\`, "tab\there", "%CURLEW_BATCH_INVOCATION_0%", ""}
				assertProgramArgs(t, wrapper, args, []string{"FOO=must-not-expand", "curlew_batch_invocation_0=user-value"})
			})
			t.Run("quoted_arguments", func(t *testing.T) {
				args := []string{`double"quote`, `"`, `"quoted words"`, `a"&b|c^d<e>f`, `\"`, `trailing\"`, `"%FOO%"`, `"!FOO!"`, `a""b`, "\u96ea\"\u00e5", `two\\"&|^<>()end`, `three\\\"&|^<>()end`, `"quoted"\`, `"quoted"\\`, `"^"`, `^"^`}
				for _, argument := range args {
					t.Run(argument, func(t *testing.T) {
						output, err := ExecuteProgram(context.Background(), wrapper, []string{"argv", argument}, []string{"FOO=must-not-expand"})
						if err != nil {
							t.Fatalf("output=%q error=%v diagnostic=%q", output, err, CommandDiagnostic(err))
						}
						var actual []string
						if err := json.Unmarshal([]byte(output), &actual); err != nil || len(actual) != 1 || actual[0] != argument {
							t.Fatalf("argv=%q want=%q decode=%v", actual, argument, err)
						}
					})
				}
				assertProgramArgs(t, wrapper, args, []string{"FOO=must-not-expand"})
			})
			t.Run("quoted_metacharacters_are_not_commands", func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "injected.txt")
				args := []string{`"& echo injected > "` + marker + `" & rem "`, `"| echo injected > "` + marker + `" & rem "`, `"<"` + marker + `"`, `"%FOO%!FOO!^&|<>()"`}
				assertProgramArgs(t, wrapper, args, []string{"FOO=must-not-expand"})
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("argument executed as a command: %v", err)
				}
			})
			t.Run("safe_exit_42", func(t *testing.T) {
				output, err := ExecuteProgram(context.Background(), wrapper, []string{"exit42"}, nil)
				assertSafeProgramFailure(t, output, err, wrapper, "private-stderr")
				if !strings.Contains(err.Error(), "code 42") || CommandDiagnostic(err) != "private-stderr\n" {
					t.Fatalf("wrapper failure lost exit or stderr: %v, diagnostic %q", err, CommandDiagnostic(err))
				}
			})
			t.Run("child_transport_collision", func(t *testing.T) {
				t.Setenv("Curlew_Batch_Invocation_0", "inherited-zero")
				output, err := ExecuteProgram(context.Background(), wrapper, []string{"env"}, []string{"CURLEW_BATCH_INVOCATION_1=child-one", "curlew_batch_invocation_0=child-zero"})
				if err != nil {
					t.Fatal(err)
				}
				var entries []string
				if err := json.Unmarshal([]byte(output), &entries); err != nil {
					t.Fatal(err)
				}
				values := make(map[string]string)
				for _, entry := range entries {
					name, value, _ := strings.Cut(entry, "=")
					values[strings.ToUpper(name)] = value
				}
				if values["CURLEW_BATCH_INVOCATION_0"] != "child-zero" || values["CURLEW_BATCH_INVOCATION_1"] != "child-one" || os.Getenv("Curlew_Batch_Invocation_0") != "inherited-zero" {
					t.Fatal("batch transport overwrote a caller environment value")
				}
			})
			t.Run("control_arguments", func(t *testing.T) {
				var args []string
				for character := byte(1); character < 32; character++ {
					if character != '\r' && character != '\n' {
						args = append(args, "before"+string(character)+"after")
					}
				}
				assertProgramArgs(t, wrapper, args, nil)
			})
			t.Run("unsupported_before_launch", func(t *testing.T) {
				marker := filepath.Join(t.TempDir(), "started")
				for _, arg := range []string{"line\nbreak", "line\rbreak", "nul\x00byte", strings.Repeat("a", 8100), strings.Repeat("\U0001f642", 4100)} {
					output, err := ExecuteProgram(context.Background(), wrapper, []string{"touch", marker, arg}, nil)
					assertSafeProgramFailure(t, output, err, wrapper, "private-injection", arg)
					if !strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "invalid") {
						t.Errorf("rejection not explicit: %v", err)
					}
				}
				output, err := ExecuteProgram(context.Background(), wrapper, []string{"touch", marker}, []string{"CURLEW_LARGE=" + strings.Repeat("a", 8100)})
				assertSafeProgramFailure(t, output, err, wrapper)
				if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unsupported argument launched wrapper: %v", err)
				}
			})
			t.Run("long_argument", func(t *testing.T) {
				assertProgramArgs(t, wrapper, []string{strings.Repeat("a", 7000)}, nil)
			})
		})
	}
}
