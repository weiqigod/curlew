//go:build windows

package uiserver

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStartEditorWindowsPreservesArguments(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	dir := filepath.Join(t.TempDir(), "editor fixtures å")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	recorder := filepath.Join(dir, "argvrecorder.exe")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", recorder, "./testdata/argvrecorder")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build argv recorder: %v\n%s", err, output)
	}

	for _, extension := range []string{".cmd", ".bat"} {
		wrapper := filepath.Join(dir, "editor"+extension)
		body := "@echo off\r\n\"%~dp0argvrecorder.exe\" %*\r\n"
		if err := os.WriteFile(wrapper, []byte(body), 0o600); err != nil {
			t.Fatalf("write wrapper: %v", err)
		}
	}
	codeWrapper := filepath.Join(dir, "code.cmd")
	body := "@echo off\r\n\"%~dp0argvrecorder.exe\" %*\r\n"
	if err := os.WriteFile(codeWrapper, []byte(body), 0o600); err != nil {
		t.Fatalf("write code wrapper: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	want := []string{
		"space value",
		`quote"inside`,
		`C:\work tree\collections\users.yaml:7`,
		"%PATH%",
		"!value!",
		"&|^<>()",
	}
	cases := []struct {
		name    string
		program string
	}{
		{name: "direct exe", program: recorder},
		{name: "cmd wrapper", program: filepath.Join(dir, "editor.cmd")},
		{name: "bat wrapper", program: filepath.Join(dir, "editor.bat")},
		{name: "PATH resolved code cmd", program: "code"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "argv.json")
			t.Setenv("CURLEW_EDITOR_ARGV_FILE", output)
			args := append([]string{test.program}, want...)
			if test.program == recorder {
				if err := startEditor(dir, args); err != nil {
					t.Fatalf("startEditor: %v", err)
				}
			} else {
				cmd, err := editorCommand(args)
				if err != nil {
					t.Fatalf("editorCommand: %v", err)
				}
				cmd.Dir = dir
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("run editor wrapper: %v\n%s", err, output)
				}
			}
			got := waitEditorArgs(t, output)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("argv = %#v, want %#v", got, want)
			}
		})
	}
}

func waitEditorArgs(t *testing.T, path string) []string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		body, err := os.ReadFile(path)
		if err == nil {
			var args []string
			if err := json.Unmarshal(body, &args); err != nil {
				t.Fatalf("decode argv: %v", err)
			}
			return args
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("editor argv file %s was not created", path)
	return nil
}
