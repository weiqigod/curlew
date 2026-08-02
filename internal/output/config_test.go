package output

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfig_Unmarshal(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want Config
	}{
		{"empty", "", Config{}},
		{"format_only", "format: json\n", Config{Format: "json"}},
		{"all_fields", "format: tap\nreport: out.txt\nevents: ev.jsonl\nverbosity: verbose\n", Config{Format: "tap", Report: "out.txt", Events: "ev.jsonl", Verbosity: "verbose"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got Config
			if err := yaml.Unmarshal([]byte(tc.yaml), &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{"nil", nil, nil},
		{"empty", &Config{}, nil},
		{"valid_format", &Config{Format: "json"}, nil},
		{"invalid_format", &Config{Format: "yaml"}, ErrUnknownFormat},
		{"valid_markdown", &Config{Format: "markdown"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("got %v, want nil", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want errors.Is(%v)", err, tc.wantErr)
			}
		})
	}
}

func TestParseVerbosity(t *testing.T) {
	tests := []struct {
		in      string
		wantV   Verbosity
		wantOK  bool
		wantErr bool
	}{
		{"", VerbosityDefault, false, false},
		{"quiet", VerbosityQuiet, true, false},
		{"normal", VerbosityDefault, true, false},
		{"verbose", VerbosityVerbose, true, false},
		{"debug", VerbosityDebug, true, false},
		{"unknown", VerbosityDefault, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			v, ok, err := ParseVerbosity(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if v != tc.wantV {
				t.Errorf("v = %v, want %v", v, tc.wantV)
			}
		})
	}
}
