// Package templates embeds default skill files that scaffold.Init can copy
// into a user project. Mirror of schemas/schemas.go (M8-001): files in
// templates/skills/<name>/... are the source of truth; this package exposes
// a thin Render helper that performs the {{curlew_version}} substitution.
package templates

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"strings"
)

//go:embed all:skills/agent/curlew
var skillFS embed.FS

// ErrUnknownSkill is returned by Render and Walk when the named skill has no
// embedded template. Sentinel so the CLI layer can branch on it cleanly.
var ErrUnknownSkill = errors.New("unknown skill")

// SupportedSkills lists the values accepted by Render, Walk, and the CLI
// layer's --skill flag. "agent" is canonical and leads the slice because the
// order is user-visible in --help and in the rejected-value error; "claude"
// is a compatibility alias kept for projects and scripts written against the
// original spelling.
//
// Every name resolves to the same payload. curlew ships one Agent Skill, not
// one per vendor: SKILL.md with name/description frontmatter is the common
// format, and .claude/skills/ is a project skill directory read by both
// Claude Code and GitHub Copilot. Adding a name here is a naming decision,
// not a packaging one — see embeddedBase.
var SupportedSkills = []string{"agent", "claude"}

// SkillRelativePath returns the in-project path where a skill's SKILL.md
// should land (e.g. ".claude/skills/curlew/SKILL.md"). Identical for every
// accepted name; kept as a function so the path can vary if a future agent
// platform expects a layout that .claude/skills/ does not satisfy.
func SkillRelativePath(_ string) string {
	return ".claude/skills/curlew/SKILL.md"
}

// SkillRootDir returns the in-project directory where a skill's files land
// (e.g. ".claude/skills/curlew"). Callers that need to enumerate files use
// this together with Walk.
func SkillRootDir(_ string) string {
	return ".claude/skills/curlew"
}

// Walk returns an iterator that yields (relativePath, body) pairs for every
// file under the named skill's embedded directory tree. relativePath is
// relative to the skill root (e.g. "SKILL.md", "variables.md"). Bodies are
// returned verbatim — {{curlew_version}} substitution is NOT applied here;
// callers (installSkill, Render) handle substitution where needed.
//
// Returns ErrUnknownSkill for any skillName outside SupportedSkills.
//
// Error handling note: iter.Seq2 cannot carry a third error value, so
// fs.WalkDir errors inside the callback cannot be surfaced to the caller.
// In practice, embedded.FS cannot fail at runtime (the files are compiled
// into the binary), so ReadFile errors are structurally impossible. If the
// walk is aborted early by yield returning false, fs.SkipAll is returned
// from the callback so the walk terminates cleanly. The fs.WalkDir return
// value is intentionally discarded for this reason.
func Walk(skillName string) (iter.Seq2[string, []byte], error) {
	base, err := embeddedBase(skillName)
	if err != nil {
		return nil, err
	}
	return func(yield func(string, []byte) bool) {
		// fs.WalkDir error is intentionally discarded: embedded.FS cannot fail
		// at runtime (files are baked into the binary); if yield returns false
		// the callback returns fs.SkipAll, terminating the walk cleanly.
		_ = fs.WalkDir(skillFS, base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			// CutPrefix strips the embedded base path to produce the skill-relative
			// name (e.g. "skills/agent/curlew/SKILL.md" → "SKILL.md"). The prefix
			// must always be present for entries under base; the !ok branch is a
			// structural invariant guard that keeps rel well-formed if the FS
			// somehow yields a path outside the expected subtree.
			rel, ok := strings.CutPrefix(p, base+"/")
			if !ok {
				// p does not have the expected prefix — use the full path so the
				// caller receives a valid (if unexpected) string rather than silently
				// creating a file at the wrong location.
				rel = p
			}
			body, rerr := skillFS.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			if !yield(rel, body) {
				return fs.SkipAll
			}
			return nil
		})
	}, nil
}

// embeddedBase returns the embedded FS path prefix for the named skill. All
// accepted names share one payload — see SupportedSkills for why.
func embeddedBase(skillName string) (string, error) {
	switch skillName {
	case "agent", "claude":
		return "skills/agent/curlew", nil
	default:
		return "", fmt.Errorf("%w %q (supported: %s)", ErrUnknownSkill, skillName, strings.Join(SupportedSkills, ", "))
	}
}

// Render returns the SKILL.md content for the named skill with
// {{curlew_version}} replaced by version. Returns ErrUnknownSkill for any
// skillName outside SupportedSkills.
//
// Backwards-compatible with pre-M19-006 callers that consumed only the entry-
// point file; the new Walk function returns all files in the skill directory.
func Render(skillName, version string) (string, error) {
	base, err := embeddedBase(skillName)
	if err != nil {
		return "", err
	}
	raw, err := skillFS.ReadFile(base + "/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("reading embedded %s/SKILL.md: %w", base, err)
	}
	return strings.ReplaceAll(string(raw), "{{curlew_version}}", version), nil
}

// IsSupportedSkill reports whether s is one of the known skill names.
func IsSupportedSkill(s string) bool {
	for _, k := range SupportedSkills {
		if s == k {
			return true
		}
	}
	return false
}
