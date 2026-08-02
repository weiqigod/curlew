package variable

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
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

// ExecuteCommand runs a shell command via /bin/sh -c and returns stdout
// with trailing newlines trimmed. Returns ErrCommandFailed on non-zero exit
// with the command, exit code, and stderr in the error message.
func ExecuteCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%w: command %q exited with code %d: %s",
				ErrCommandFailed, command, exitErr.ExitCode(), strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%w: command %q: %w", ErrCommandFailed, command, err)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}
