package output

import "testing"

func TestVerbosity(t *testing.T) {
	tests := []struct {
		name    string
		v       Verbosity
		wantStr string
	}{
		{"quiet string", VerbosityQuiet, "quiet"},
		{"default string", VerbosityDefault, "default"},
		{"verbose string", VerbosityVerbose, "verbose"},
		{"debug string", VerbosityDebug, "debug"},
		{"unknown value", Verbosity(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.String(); got != tt.wantStr {
				t.Errorf("got %q, want %q", got, tt.wantStr)
			}
		})
	}
}

func TestVerbosityOrdering(t *testing.T) {
	if VerbosityQuiet >= VerbosityDefault {
		t.Error("quiet must be less than default")
	}
	if VerbosityDefault >= VerbosityVerbose {
		t.Error("default must be less than verbose")
	}
	if VerbosityVerbose >= VerbosityDebug {
		t.Error("verbose must be less than debug")
	}
}
