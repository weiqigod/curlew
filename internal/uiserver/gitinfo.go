package uiserver

import (
	"os"
	"path/filepath"
	"strings"
)

// readGitInfo reads .git/HEAD directly — no shell-out (spec §8.2): works
// without a git binary, deterministic, no per-run process spawn. Walks up
// from root for .git; handles worktree gitdir files, symbolic refs via loose
// refs and packed-refs, and detached HEAD. Any failure → nil.
func readGitInfo(root string) *gitJSON {
	gitDir := findGitDir(root)
	if gitDir == "" {
		return nil
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return nil
	}
	content := strings.TrimSpace(string(head))

	if ref, ok := strings.CutPrefix(content, "ref: "); ok {
		branch := strings.TrimPrefix(ref, "refs/heads/")
		commit := resolveRef(gitDir, ref)
		g := &gitJSON{Branch: &branch}
		if commit != "" {
			g.Commit = &commit
		}
		return g
	}
	// Detached HEAD: content is the commit hash, branch is null.
	if len(content) == 40 {
		commit := content
		return &gitJSON{Commit: &commit}
	}
	return nil
}

// findGitDir walks up from dir looking for .git (directory, or a worktree
// "gitdir: <path>" file).
func findGitDir(dir string) string {
	for {
		candidate := filepath.Join(dir, ".git")
		fi, err := os.Stat(candidate)
		if err == nil {
			if fi.IsDir() {
				return candidate
			}
			// Worktree: .git is a file "gitdir: <path>".
			data, err := os.ReadFile(candidate)
			if err == nil {
				line := strings.TrimSpace(string(data))
				if p, ok := strings.CutPrefix(line, "gitdir: "); ok {
					if !filepath.IsAbs(p) {
						p = filepath.Join(dir, p)
					}
					return p
				}
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// resolveRef resolves a symbolic ref to a commit hash via the loose ref file
// or packed-refs. Best-effort; "" when unresolvable.
func resolveRef(gitDir, ref string) string {
	if data, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		return strings.TrimSpace(string(data))
	}
	// Worktree git dirs delegate refs to the common dir.
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common := strings.TrimSpace(string(data))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		if data, err := os.ReadFile(filepath.Join(common, filepath.FromSlash(ref))); err == nil {
			return strings.TrimSpace(string(data))
		}
		gitDir = common
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[1] == ref {
			return parts[0]
		}
	}
	return ""
}
