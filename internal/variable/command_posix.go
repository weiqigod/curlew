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

func trimCommandOutput(output string) string {
	return strings.TrimRight(output, "\n")
}
