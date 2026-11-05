//go:build windows

package uiserver

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

func startEditor(dir string, args []string) error {
	cmd, err := editorCommand(args)
	if err != nil {
		return err
	}
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func editorCommand(args []string) (*exec.Cmd, error) {
	resolved, err := exec.LookPath(args[0])
	if err != nil {
		return nil, err
	}
	extension := filepath.Ext(resolved)
	if !strings.EqualFold(extension, ".cmd") && !strings.EqualFold(extension, ".bat") {
		return exec.Command(resolved, args[1:]...), nil //nolint:gosec // explicitly configured local editor
	}

	invocation := "\"" + resolved + "\""
	for _, arg := range args[1:] {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return nil, fmt.Errorf("editor batch arguments cannot contain NUL, CR or LF")
		}
		invocation += " " + quoteEditorBatchArgument(arg)
	}
	environment := os.Environ()
	transport := editorTransportKey(environment)
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if len(utf16.Encode([]rune(shell+invocation+transport)))+32 > 8000 {
		return nil, fmt.Errorf("editor batch invocation exceeds 8000 UTF-16 units")
	}
	cmd := exec.Command(shell)
	cmd.Env = append(environment, transport+"="+invocation)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: "\"" + shell + "\" /d /v:off /s /c \"%" + transport + "%\"",
	}
	return cmd, nil
}

func editorTransportKey(environment []string) string {
	for index := 0; ; index++ {
		candidate := fmt.Sprintf("CURLEW_EDITOR_INVOCATION_%d", index)
		found := false
		for _, entry := range environment {
			name, _, _ := strings.Cut(entry, "=")
			if strings.EqualFold(name, candidate) {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
}

func quoteEditorBatchArgument(value string) string {
	var encoded strings.Builder
	encoded.WriteByte('"')
	backslashes := 0
	for _, character := range value {
		if character == '\\' {
			backslashes++
			continue
		}
		if character == '"' {
			encoded.WriteString(strings.Repeat("\\", backslashes*2))
			encoded.WriteString("\"\\^^^\"\"")
		} else {
			encoded.WriteString(strings.Repeat("\\", backslashes))
			encoded.WriteRune(character)
		}
		backslashes = 0
	}
	encoded.WriteString(strings.Repeat("\\", backslashes*2))
	encoded.WriteByte('"')
	return encoded.String()
}
