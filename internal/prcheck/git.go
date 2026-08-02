package prcheck

import (
	"os/exec"
	"strings"
)

// DetectGitSha runs `git rev-parse --short HEAD` in dir and returns the short
// SHA on success, or an empty string if dir is not a git repository or if git
// is not installed.
func DetectGitSha(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
