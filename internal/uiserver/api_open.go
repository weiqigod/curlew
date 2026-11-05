package uiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
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

	args, err := buildEditorArgs(editor, abs, body.Line)
	if err != nil {
		writeAPIError(w, http.StatusConflict, "no_editor", "invalid editor command: "+err.Error(),
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}
	if len(args) == 0 {
		writeAPIError(w, http.StatusConflict, "no_editor", "editor command is empty",
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}
	if err := startEditor(s.opts.Root, args); err != nil {
		writeAPIError(w, http.StatusConflict, "no_editor", "could not launch editor: "+err.Error(),
			"set ui.editor in curlew.yaml or $CURLEW_EDITOR", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// buildEditorArgs executes the command template: {file}/{line} placeholders
// substituted when present, else "<cmd> <abs-path>:<line>" appended (§4.13).
func buildEditorArgs(template, absFile string, line int) ([]string, error) {
	fields, err := splitEditorTemplate(template)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
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
	return out, nil
}

func splitEditorTemplate(template string) ([]string, error) {
	if strings.ContainsAny(template, "\x00\r\n") {
		return nil, fmt.Errorf("NUL, CR and LF are not allowed")
	}
	var args []string
	var current strings.Builder
	var quote rune
	started := false
	flush := func() {
		if started {
			args = append(args, current.String())
			current.Reset()
			started = false
		}
	}
	for _, character := range template {
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			started = true
			continue
		}
		switch {
		case character == '\'' || character == '"':
			quote = character
			started = true
		case unicode.IsSpace(character):
			flush()
		default:
			current.WriteRune(character)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unmatched %q quote", quote)
	}
	flush()
	return args, nil
}
