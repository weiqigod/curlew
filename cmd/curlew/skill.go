package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/weiqigod/curlew/internal/skillinstall"
)

func skillCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "update") {
		_, _ = fmt.Fprintln(stderr, "Usage: curlew skill <install|update> --agent <codex|claude|copilot> [dir]")
		return 1
	}
	opts := skillinstall.Options{Version: resolvedVersion, Update: args[0] == "update"}
	var positional []string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printSkillHelpTo(stdout)
			return 0
		case "--agent":
			i++
			if i >= len(args) || strings.HasPrefix(args[i], "-") {
				_, _ = fmt.Fprintln(stderr, "Error: --agent requires codex, claude or copilot")
				return 1
			}
			opts.Agent = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				_, _ = fmt.Fprintf(stderr, "Error: unknown option %s\n", args[i])
				return 1
			}
			positional = append(positional, args[i])
		}
	}
	if opts.Agent == "" || len(positional) > 1 {
		_, _ = fmt.Fprintln(stderr, "Error: choose --agent <codex|claude|copilot> and at most one project directory")
		return 1
	}
	if len(positional) == 1 {
		opts.Dir = positional[0]
	}
	root, err := skillinstall.Install(opts)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 3
	}
	_, _ = fmt.Fprintf(stdout, "Curlew skill ready: %s\nProject configuration unchanged. Read SKILL.md to start.\n", root)
	return 0
}

func printSkillHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, `Usage: curlew skill <install|update> --agent <codex|claude|copilot> [dir]

Install the bundled skill into a project (default: current directory).
  codex    .agents/skills/curlew
  claude   .claude/skills/curlew
  copilot  .github/skills/curlew

install adopts identical files; update replaces unchanged managed files.
Local edits cause a conflict before any writes. Custom files are preserved.
Keep .curlew-skill.json with the skill for safe updates. For conflicts, install
into a temporary directory and merge your changes manually. No force option.
Neither command changes curlew.yaml, collections or output configuration.
Exit codes: 0 success, 1 usage error, 3 installation/update error.
  --help, -h  Show this help message`)
}
