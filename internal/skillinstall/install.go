// Package skillinstall installs the embedded agent skill into a project.
package skillinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/weiqigod/curlew/templates"
)

const manifestName = ".curlew-skill.json"

// Options selects the project, agent and safe update mode.
type Options struct {
	Dir     string
	Agent   string
	Version string
	Update  bool
}
type manifest struct {
	Schema  int               `json:"schema"`
	Version string            `json:"curlew_version"`
	Files   map[string]string `json:"files"`
}
type entry struct {
	path string
	body []byte
}

// Install copies the bundled skill. Conflicts are detected before any writes.
// Only unchanged files recorded by an earlier install can be updated. Unknown
// files are retained, and identical legacy files can be adopted safely.
func Install(opts Options) (string, error) {
	targets := map[string]string{"codex": ".agents", "claude": ".claude", "copilot": ".github"}
	target, ok := targets[opts.Agent]
	if !ok {
		return "", fmt.Errorf("unknown agent %q (supported: codex, claude, copilot)", opts.Agent)
	}
	dir := opts.Dir
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving project directory: %w", err)
	}
	// Permit project ancestors such as macOS /tmp, but reject symlinks inside
	// the selected agent destination. No manifest path controls writes.
	root := filepath.Join(abs, target, "skills", "curlew")
	if err = checkPath(abs, root, true); err != nil {
		return "", err
	}
	statePath := filepath.Join(root, manifestName)
	if err = checkPath(abs, statePath, false); err != nil {
		return "", err
	}
	old := manifest{}
	data, err := os.ReadFile(statePath)
	if err == nil {
		if err = json.Unmarshal(data, &old); err != nil {
			return "", fmt.Errorf("reading skill manifest: %w", err)
		}
		if old.Schema != 1 || old.Files == nil {
			return "", fmt.Errorf("invalid skill manifest in %s", statePath)
		}
		for name, hash := range old.Files {
			decoded, e := hex.DecodeString(hash)
			if !fs.ValidPath(name) || strings.Contains(name, "\\") || e != nil || len(decoded) != sha256.Size {
				return "", fmt.Errorf("invalid skill manifest entry %q", name)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("reading skill manifest: %w", err)
	}
	if opts.Update {
		if _, err = os.Lstat(filepath.Join(root, "SKILL.md")); err != nil {
			return "", fmt.Errorf("skill not installed; use skill install first: %w", err)
		}
	}
	seq, err := templates.Walk("agent")
	if err != nil {
		return "", fmt.Errorf("loading skill: %w", err)
	}
	next := manifest{Schema: 1, Version: opts.Version, Files: map[string]string{}}
	var writes []entry
	var conflicts []string
	for rel, body := range seq {
		if strings.HasSuffix(rel, ".md") {
			body = []byte(strings.ReplaceAll(string(body), "{{curlew_version}}", opts.Version))
		}
		dest := filepath.Join(root, filepath.FromSlash(rel))
		if err = checkPath(abs, dest, false); err != nil {
			return "", err
		}
		want := digest(body)
		next.Files[rel] = want
		current, readErr := os.ReadFile(dest)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return "", fmt.Errorf("reading %s: %w", dest, readErr)
		}
		if readErr == nil {
			have := digest(current)
			if have == want {
				continue
			}
			if !opts.Update || old.Files[rel] != have {
				conflicts = append(conflicts, rel)
				continue
			}
		}
		writes = append(writes, entry{dest, body})
	}
	if len(conflicts) > 0 {
		return "", fmt.Errorf("skill conflict in %s: %s; no files changed. Use skill update for unchanged managed files. For local edits or legacy copies, install to a temporary directory and merge the diff manually", root, strings.Join(conflicts, ", "))
	}
	state, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding skill manifest: %w", err)
	}
	state = append(state, '\n')
	if string(state) != string(data) {
		writes = append(writes, entry{statePath, state})
	}
	for _, w := range writes {
		if err = writeAtomic(w.path, w.body); err != nil {
			return "", err
		}
	}
	return root, nil
}
func digest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

// checkPath rejects links and non-directory parents below the project root.
// The caller owns the project; concurrent hostile filesystem mutation is outside
// this local installer contract.
func checkPath(base, dest string, directory bool) error {
	rel, err := filepath.Rel(base, dest)
	if err != nil {
		return fmt.Errorf("resolving skill path: %w", err)
	}
	current := base
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking skill path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in skill destination: %s", current)
		}
		wantDir := i < len(parts)-1 || directory
		if wantDir && !info.IsDir() || !wantDir && !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected file type in skill destination: %s", current)
		}
	}
	return nil
}

func writeAtomic(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("creating skill directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".curlew-install-*")
	if err != nil {
		return fmt.Errorf("creating skill file: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(body); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing skill file: %w", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("closing skill file: %w", err)
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replacing skill file %s: %w", path, err)
	}
	return nil
}
