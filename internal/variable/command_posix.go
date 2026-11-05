//go:build !windows

package variable

import (
	"context"
	"os/exec"
	"strings"
)

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}

func programCommand(ctx context.Context, program string, args, env []string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = env
	return cmd, nil
}

func programEnvironmentKey(name string) string {
	return name
}

func trimCommandOutput(output string) string {
	return strings.TrimRight(output, "\n")
}
