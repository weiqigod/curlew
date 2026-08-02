package uiserver

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// handleOpen serves POST /api/v1/open (spec §4.13): launches the user's
// editor at file:line, fire-and-forget. $VISUAL/$EDITOR are deliberately not
// consulted — they typically name terminal editors, which cannot be sensibly
// spawned from a detached server process.
func (s *Server) handleOpen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		File string `json:"file"`
		Line int    `json:"line"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "malformed JSON body", "", nil)
		return
	}
	abs := s.resolveInRoot(body.File)
	if abs == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "file outside project root", "", nil)
		return
	}
	if body.Line < 1 {
		body.Line = 1
	}

	editor := s.opts.EditorCommand
	if editor == "" {
		// Last-resort attempt: VS Code when on PATH.
		if _, err := exec.LookPath("code"); err == nil {
			editor = "code --goto {file}:{line}"
		}
	}
	if editor == "" {
		writeAPIError(w, http.StatusConflict, "no_editor", "no editor configured",
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}

	args := buildEditorArgs(editor, abs, body.Line)
	if len(args) == 0 {
		writeAPIError(w, http.StatusConflict, "no_editor", "editor command is empty",
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // user-configured editor command, localhost-only server
	cmd.Dir = s.opts.Root
	if err := cmd.Start(); err != nil {
		writeAPIError(w, http.StatusConflict, "no_editor", "could not launch editor: "+err.Error(),
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}
	go func() { _ = cmd.Wait() }() // reap; non-zero editor exit is not detected
	w.WriteHeader(http.StatusNoContent)
}

// buildEditorArgs executes the command template: {file}/{line} placeholders
// substituted when present, else "<cmd> <abs-path>:<line>" appended (§4.13).
func buildEditorArgs(template, absFile string, line int) []string {
	fields := strings.Fields(template)
	if len(fields) == 0 {
		return nil
	}
	hasPlaceholder := strings.Contains(template, "{file}") || strings.Contains(template, "{line}")
	out := make([]string, 0, len(fields)+1)
	for _, f := range fields {
		f = strings.ReplaceAll(f, "{file}", absFile)
		f = strings.ReplaceAll(f, "{line}", strconv.Itoa(line))
		out = append(out, f)
	}
	if !hasPlaceholder {
		out = append(out, absFile+":"+strconv.Itoa(line))
	}
	return out
}
