package prcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectGitSha(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns dir to pass to DetectGitSha
		wantLen int                       // 0 = expect empty string, >0 = expect non-empty
	}{
		{
			name: "not_a_git_repo",
			setup: func(t *testing.T) string {
				return t.TempDir() // fresh dir with no .git
			},
			wantLen: 0,
		},
		{
			name: "git_binary_missing",
			setup: func(t *testing.T) string {
				// Override PATH to exclude git
				t.Setenv("PATH", t.TempDir())
				return t.TempDir()
			},
			wantLen: 0,
		},
		{
			name: "valid_git_repo",
			setup: func(t *testing.T) string {
				// Create a real git repo with a commit
				dir := t.TempDir()
				run := func(args ...string) {
					cmd := exec.Command("git", args...)
					cmd.Dir = dir
					cmd.Env = append(os.Environ(),
						"GIT_AUTHOR_NAME=Test",
						"GIT_AUTHOR_EMAIL=test@example.com",
						"GIT_COMMITTER_NAME=Test",
						"GIT_COMMITTER_EMAIL=test@example.com",
					)
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("git %v failed: %v\n%s", args, err, out)
					}
				}
				run("init")
				run("config", "user.email", "test@example.com")
				run("config", "user.name", "Test")
				// Create a file and commit
				f := filepath.Join(dir, "README.md")
				if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
					t.Fatalf("write file: %v", err)
				}
				run("add", ".")
				run("commit", "-m", "initial")
				return dir
			},
			wantLen: 7, // short SHA is at least 7 chars
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			sha := DetectGitSha(dir)
			if tc.wantLen == 0 {
				if sha != "" {
					t.Errorf("DetectGitSha() = %q; want empty string", sha)
				}
			} else {
				if len(sha) < tc.wantLen {
					t.Errorf("DetectGitSha() = %q (len %d); want len >= %d", sha, len(sha), tc.wantLen)
				}
			}
		})
	}
}
