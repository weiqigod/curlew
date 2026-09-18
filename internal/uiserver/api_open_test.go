package uiserver

import (
	"reflect"
	"testing"
)

func TestBuildEditorArgs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		file     string
		line     int
		want     []string
		wantErr  bool
	}{
		{
			name:     "quoted Windows executable and file placeholder",
			template: `"C:\Program Files\Microsoft VS Code\Code.exe" --goto "{file}:{line}"`,
			file:     `C:\work tree\collections\users.yaml`,
			line:     17,
			want:     []string{`C:\Program Files\Microsoft VS Code\Code.exe`, "--goto", `C:\work tree\collections\users.yaml:17`},
		},
		{
			name:     "single quoted argument",
			template: `code --reuse-window '{file}:{line}'`,
			file:     `/tmp/work tree/users.yaml`,
			line:     4,
			want:     []string{"code", "--reuse-window", "/tmp/work tree/users.yaml:4"},
		},
		{
			name:     "no placeholder appends location",
			template: `code --wait`,
			file:     `C:\work\users.yaml`,
			line:     2,
			want:     []string{"code", "--wait", `C:\work\users.yaml:2`},
		},
		{
			name:     "metacharacters remain argument data",
			template: `editor "value & | ^ < > ( )" {file}`,
			file:     `C:\work\users.yaml`,
			line:     1,
			want:     []string{"editor", "value & | ^ < > ( )", `C:\work\users.yaml`},
		},
		{name: "empty template", template: " \t ", file: "file.yaml", line: 1, want: nil},
		{name: "unmatched quote", template: `"C:\Program Files\Code.exe`, file: "file.yaml", line: 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildEditorArgs(test.template, test.file, test.line)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildEditorArgs: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("args = %#v, want %#v", got, test.want)
			}
		})
	}
}
