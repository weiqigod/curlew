package variable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"
)

// ErrCommandFailed indicates a from_command execution returned a non-zero exit code.
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
func ExecuteCommand(ctx context.Context, command string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", &commandFailure{cause: err, reason: err.Error()}
	}
	cmd := shellCommand(ctx, command)
	return executeCapturedCommand(ctx, cmd)
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
