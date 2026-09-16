package variable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ErrCommandFailed indicates command validation, execution, or output validation failed.
var ErrCommandFailed = errors.New("from_command execution failed")

// CommandCache stores command output values with expiration.
type CommandCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	value   string
	expires time.Time
}

// NewCommandCache returns an initialised empty cache.
func NewCommandCache() *CommandCache {
	return &CommandCache{entries: make(map[string]cacheEntry)}
}

// Get returns the cached value and true if the key exists and has not expired.
func (c *CommandCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, key)
		return "", false
	}
	return e.value, true
}

// Set stores a value with the given TTL in seconds. Zero TTL is a no-op.
func (c *CommandCache) Set(key, value string, ttlSeconds int) {
	if ttlSeconds <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		value:   value,
		expires: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
}

type commandFailure struct {
	stdout string
	stderr string
	cause  error
	reason string
}

func (failure *commandFailure) Error() string {
	return ErrCommandFailed.Error() + ": " + failure.reason
}

func (failure *commandFailure) Is(target error) bool {
	return target == ErrCommandFailed
}

func (failure *commandFailure) Unwrap() error {
	return failure.cause
}

// CommandDiagnostic returns captured UTF-8 stderr from a command failure.
// WARNING: This may contain secrets. Use only for internal classification; never render it.
func CommandDiagnostic(err error) string {
	var failure *commandFailure
	if errors.As(err, &failure) && utf8.ValidString(failure.stderr) {
		return failure.stderr
	}
	return ""
}

// ExecuteCommand runs a platform shell script and returns UTF-8 stdout with
// trailing newlines trimmed. Failure messages omit scripts and captured output.
// Execution is capped at 30 seconds, or the caller's earlier deadline.
func ExecuteCommand(ctx context.Context, command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", &commandFailure{cause: err, reason: err.Error()}
	}
	cmd := shellCommand(ctx, command)
	return executeCapturedCommand(ctx, cmd)
}

// ExecuteProgram runs a program with structured arguments and child environment overrides.
// Overrides inherit the parent environment; later values win (case-insensitively on Windows).
// Execution is capped at 30 seconds, or the caller's earlier deadline. Output must be UTF-8.
// Windows batch files use cmd.exe for native %* forwarding: quotes and CR/LF in arguments
// are unsupported, and the invocation and each environment entry are limited to 8000 UTF-16 units.
func ExecuteProgram(ctx context.Context, program string, args, env []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", &commandFailure{cause: err, reason: err.Error()}
	}
	if program == "" || !validProgramString(program) {
		return "", &commandFailure{reason: "invalid program name"}
	}
	for _, arg := range args {
		if !validProgramString(arg) {
			return "", &commandFailure{reason: "invalid program argument"}
		}
	}
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if !found || name == "" || !validProgramString(entry) {
			return "", &commandFailure{reason: "invalid program environment override"}
		}
	}
	cmd, err := programCommand(ctx, program, args, mergeProgramEnvironment(os.Environ(), env))
	if err != nil {
		return "", &commandFailure{cause: err, reason: "program could not be prepared: " + programPreparationReason(err)}
	}
	return executeCapturedCommand(ctx, cmd)
}

func validProgramString(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func programPreparationReason(err error) string {
	var failure *commandFailure
	if errors.As(err, &failure) {
		return failure.reason
	}
	return "executable lookup failed"
}

func mergeProgramEnvironment(inherited, overrides []string) []string {
	merged := make([]string, 0, len(inherited)+len(overrides))
	positions := make(map[string]int, len(inherited)+len(overrides))
	for _, entries := range [][]string{inherited, overrides} {
		for _, entry := range entries {
			separator := strings.IndexByte(entry, '=')
			if separator == 0 {
				separator = strings.IndexByte(entry[1:], '=') + 1
			}
			if separator < 1 {
				continue
			}
			key := programEnvironmentKey(entry[:separator])
			if position, found := positions[key]; found {
				merged[position] = entry
			} else {
				positions[key] = len(merged)
				merged = append(merged, entry)
			}
		}
	}
	return merged
}

func executeCapturedCommand(ctx context.Context, cmd *exec.Cmd) (string, error) {
	cmd.WaitDelay = time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := runContainedCommand(cmd)
	if err != nil || !utf8.Valid(stdout.Bytes()) || !utf8.Valid(stderr.Bytes()) {
		failure := &commandFailure{
			stdout: stdout.String(), stderr: stderr.String(), cause: err,
			reason: "process could not start or complete",
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			failure.reason = fmt.Sprintf("process exited with code %d", exitErr.ExitCode())
		}
		if !utf8.Valid(stdout.Bytes()) || !utf8.Valid(stderr.Bytes()) {
			failure.reason = "process output is not valid UTF-8"
		}
		if err != nil && ctx.Err() != nil {
			failure.cause = errors.Join(err, ctx.Err())
			failure.reason = ctx.Err().Error()
		}
		return "", failure
	}

	return trimCommandOutput(stdout.String()), nil
}
