package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name  string
		input *JSONOutput
		check func(t *testing.T, data []byte)
	}{
		{
			name: "valid JSON output",
			input: &JSONOutput{
				Name:       "My Suite",
				Status:     "passed",
				DurationMs: 123,
				Requests:   []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatalf("not valid JSON: %s", data)
				}
			},
		},
		{
			name: "empty assertions array not null",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "req1",
						Status:     "passed",
						Method:     "GET",
						URL:        "https://example.com",
						Assertions: []JSONAssertion{},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"assertions": []`)) {
					t.Fatalf("expected empty assertions array, got: %s", data)
				}
			},
		},
		{
			name: "all top-level fields present",
			input: &JSONOutput{
				Name:       "Suite",
				Status:     "passed",
				DurationMs: 500,
				Requests:   []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"name"`, `"status"`, `"duration_ms"`, `"requests"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing field %s in: %s", field, data)
					}
				}
			},
		},
		{
			name: "assertion failure fields present",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "failed",
				Requests: []JSONRequest{
					{
						Name:   "req",
						Status: "failed",
						Method: "GET",
						URL:    "https://example.com",
						Assertions: []JSONAssertion{
							{
								Type:     "status",
								Operator: "equals",
								Expected: "200",
								Actual:   "404",
								Passed:   false,
							},
						},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"expected"`, `"actual"`, `"operator"`, `"passed"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing assertion field %s", field)
					}
				}
				if bytes.Contains(data, []byte(`"passed": true`)) {
					t.Error("expected passed:false, got true")
				}
			},
		},
		{
			name: "error with hint",
			input: &JSONOutput{
				Name:     "Suite",
				Status:   "error",
				Errors:   []JSONError{{Message: "connection refused", Hint: "check server"}},
				Requests: []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"errors"`)) {
					t.Fatal("missing errors field")
				}
				if !bytes.Contains(data, []byte(`"hint"`)) {
					t.Fatal("missing hint field")
				}
			},
		},
		{
			name: "status_code present even when zero (error request)",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "failed",
				Requests: []JSONRequest{
					{
						Name:       "req",
						Status:     "error",
						Method:     "GET",
						URL:        "http://127.0.0.1:1/fail",
						StatusCode: 0,
						Assertions: []JSONAssertion{},
						Error:      &JSONError{Message: "connection refused"},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"status_code": 0`)) {
					t.Errorf("expected status_code: 0 in output for error request, got: %s", data)
				}
			},
		},
		{
			name: "retry_count omitted when zero",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "req1",
						Status:     "passed",
						Method:     "GET",
						URL:        "https://example.com",
						RetryCount: 0,
						Assertions: []JSONAssertion{},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				if bytes.Contains(data, []byte(`"retry_count"`)) {
					t.Errorf("expected retry_count to be omitted when 0, got: %s", data)
				}
			},
		},
		{
			name: "retry_count present when greater than zero",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "req1",
						Status:     "passed",
						Method:     "GET",
						URL:        "https://example.com",
						RetryCount: 2,
						Assertions: []JSONAssertion{},
					},
				},
			},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"retry_count": 2`)) {
					t.Errorf("expected retry_count: 2 in output, got: %s", data)
				}
			},
		},
		{
			name: "no extraneous text — starts with { ends with newline",
			input: &JSONOutput{
				Name:     "Suite",
				Status:   "passed",
				Requests: []JSONRequest{},
			},
			check: func(t *testing.T, data []byte) {
				trimmed := bytes.TrimSpace(data)
				if !bytes.HasPrefix(trimmed, []byte("{")) {
					t.Fatalf("expected output to start with '{', got: %s", data[:10])
				}
				if !bytes.HasSuffix(trimmed, []byte("}")) {
					t.Fatalf("expected output to end with '}', got tail: %s", data[len(data)-10:])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tc.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			tc.check(t, buf.Bytes())
		})
	}
}

func intPtr(v int) *int { return &v }

func TestWriteJSON_WaveIndex(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"wave_index present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						WaveIndex: intPtr(0), Assertions: []JSONAssertion{},
					},
				},
			},
			[]string{`"wave_index": 0`},
			nil,
		},
		{
			"wave_index omitted when nil",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						Assertions: []JSONAssertion{},
					},
				},
			},
			nil,
			[]string{`"wave_index"`},
		},
		{
			"wave_index 1 present",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						WaveIndex: intPtr(1), Assertions: []JSONAssertion{},
					},
				},
			},
			[]string{`"wave_index": 1`},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestWriteJSON_ParallelExecution(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"parallel_execution present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				ParallelExecution: &ParallelExecutionJSON{
					WaveCount: 3, MaxParallelism: 2, SpeedupFactor: 1.5,
				},
			},
			[]string{`"parallel_execution"`, `"wave_count": 3`, `"max_parallelism": 2`, `"speedup_factor": 1.5`},
			nil,
		},
		{
			"parallel_execution omitted when nil",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
			},
			nil,
			[]string{`"parallel_execution"`},
		},
		{
			"speedup_factor omitted when zero",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				ParallelExecution: &ParallelExecutionJSON{
					WaveCount: 1, MaxParallelism: 3,
				},
			},
			[]string{`"wave_count": 1`},
			[]string{`"speedup_factor"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestWriteValidationJSON(t *testing.T) {
	tests := []struct {
		name  string
		input *ValidationJSONOutput
		check func(t *testing.T, data []byte)
	}{
		{
			name: "empty_files_produces_valid_json",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{},
				Valid: true,
			},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatalf("not valid JSON: %s", data)
				}
			},
		},
		{
			name: "single_valid_file_no_issues",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{
					{File: "col.yaml", Valid: true, Issues: []ValidationIssueJSON{}},
				},
				Valid: true,
			},
			check: func(t *testing.T, data []byte) {
				var out ValidationJSONOutput
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !out.Valid {
					t.Error("expected valid=true")
				}
				if len(out.Files) != 1 {
					t.Errorf("expected 1 file, got %d", len(out.Files))
				}
				if len(out.Files[0].Issues) != 0 {
					t.Errorf("expected no issues, got %d", len(out.Files[0].Issues))
				}
			},
		},
		{
			name: "single_file_with_errors",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{
					{
						File:  "bad.yaml",
						Valid: false,
						Issues: []ValidationIssueJSON{
							{Severity: "error", Line: 3, Message: "invalid YAML", Hint: "fix it"},
						},
					},
				},
				Valid: false,
			},
			check: func(t *testing.T, data []byte) {
				var out ValidationJSONOutput
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if out.Valid {
					t.Error("expected valid=false")
				}
				iss := out.Files[0].Issues[0]
				if iss.Severity != "error" {
					t.Errorf("severity = %q, want %q", iss.Severity, "error")
				}
				if iss.Line != 3 {
					t.Errorf("line = %d, want 3", iss.Line)
				}
			},
		},
		{
			name: "single_file_with_warnings",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{
					{
						File:  "warn.yaml",
						Valid: true,
						Issues: []ValidationIssueJSON{
							{Severity: "warning", Message: "undefined variable"},
						},
					},
				},
				Valid: true,
			},
			check: func(t *testing.T, data []byte) {
				var out ValidationJSONOutput
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if !out.Valid {
					t.Error("expected valid=true for warnings only")
				}
				if out.Files[0].Issues[0].Severity != "warning" {
					t.Errorf("severity = %q, want warning", out.Files[0].Issues[0].Severity)
				}
			},
		},
		{
			name: "multiple_files_mixed_validity",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{
					{File: "ok.yaml", Valid: true, Issues: []ValidationIssueJSON{}},
					{File: "bad.yaml", Valid: false, Issues: []ValidationIssueJSON{
						{Severity: "error", Message: "syntax error"},
					}},
				},
				Valid: false,
			},
			check: func(t *testing.T, data []byte) {
				var out ValidationJSONOutput
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if len(out.Files) != 2 {
					t.Errorf("expected 2 files, got %d", len(out.Files))
				}
				if out.Valid {
					t.Error("expected valid=false when any file is invalid")
				}
			},
		},
		{
			name: "line_omitted_when_zero",
			input: &ValidationJSONOutput{
				Files: []ValidationFileJSON{
					{
						File:  "col.yaml",
						Valid: false,
						Issues: []ValidationIssueJSON{
							{Severity: "error", Line: 0, Message: "no line info"},
						},
					},
				},
				Valid: false,
			},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatal("not valid JSON")
				}
				// Line 0 should be omitted from JSON (omitempty)
				if bytes.Contains(data, []byte(`"line"`)) {
					t.Errorf("expected \"line\" key to be omitted for zero value, got: %s", data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteValidationJSON(&buf, tc.input); err != nil {
				t.Fatalf("WriteValidationJSON error: %v", err)
			}
			tc.check(t, buf.Bytes())
		})
	}
}

func TestGuardRailJSON_Serialization(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name: "guard_rail field present when set",
			input: &JSONOutput{
				Name:     "Suite",
				Status:   "guard_rail",
				Requests: []JSONRequest{},
				GuardRail: &GuardRailJSON{
					LimitExceeded:    true,
					RequestsExecuted: 1000,
					Limit:            1000,
					Message:          "Request limit exceeded.",
				},
			},
			wantSubstr: []string{`"guard_rail"`, `"limit_exceeded": true`, `"requests_executed": 1000`, `"limit": 1000`},
		},
		{
			name: "guard_rail omitted when nil",
			input: &JSONOutput{
				Name:     "Suite",
				Status:   "passed",
				Requests: []JSONRequest{},
			},
			wantAbsent: []string{`"guard_rail"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			if !json.Valid(data) {
				t.Fatalf("not valid JSON: %s", data)
			}
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestWriteVaultListJSON(t *testing.T) {
	tests := []struct {
		name  string
		input *VaultListJSONOutput
		check func(t *testing.T, data []byte)
	}{
		{
			name:  "valid JSON output",
			input: &VaultListJSONOutput{Provider: "aws-secrets-manager", Keys: []VaultKeyJSON{{Name: "k", Path: "p"}}},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatalf("not valid JSON: %s", data)
				}
			},
		},
		{
			name:  "all fields present",
			input: &VaultListJSONOutput{Provider: "azure-key-vault", Keys: []VaultKeyJSON{{Name: "a", Path: "b"}}},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"provider"`, `"keys"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing field %s in: %s", field, data)
					}
				}
			},
		},
		{
			name:  "empty keys is empty array",
			input: &VaultListJSONOutput{Provider: "aws-secrets-manager", Keys: []VaultKeyJSON{}},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"keys": []`)) {
					t.Errorf("expected empty keys array, got: %s", data)
				}
			},
		},
		{
			name:  "keys include name and path",
			input: &VaultListJSONOutput{Provider: "aws-secrets-manager", Keys: []VaultKeyJSON{{Name: "api_key", Path: "prod/key"}}},
			check: func(t *testing.T, data []byte) {
				if !bytes.Contains(data, []byte(`"name": "api_key"`)) {
					t.Errorf("missing key name in: %s", data)
				}
				if !bytes.Contains(data, []byte(`"path": "prod/key"`)) {
					t.Errorf("missing key path in: %s", data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteVaultListJSON(&buf, tc.input); err != nil {
				t.Fatalf("WriteVaultListJSON error: %v", err)
			}
			tc.check(t, buf.Bytes())
		})
	}
}

func TestWriteJSON_DataDriven(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"data_driven present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{
					{Type: "data_driven", Name: "Create Users", TotalIterations: 100, PassedIterations: 97, FailedIterations: 3, TotalDurationMs: 13500, AvgDurationMs: 135},
				},
			},
			[]string{`"data_driven"`, `"total_iterations": 100`, `"passed_iterations": 97`, `"failed_iterations": 3`, `"average_duration_ms": 135`},
			nil,
		},
		{
			"data_driven omitted when nil",
			&JSONOutput{Name: "Suite", Status: "passed", Requests: []JSONRequest{}},
			nil,
			[]string{`"data_driven"`},
		},
		{
			"data_driven type is data_driven",
			&JSONOutput{
				Name: "Suite", Status: "passed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{Type: "data_driven", Name: "Test", TotalIterations: 5, PassedIterations: 5}},
			},
			[]string{`"type": "data_driven"`},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestWriteInfoJSON(t *testing.T) {
	tests := []struct {
		name  string
		input *InfoJSONOutput
		check func(t *testing.T, data []byte)
	}{
		{
			name: "valid_JSON_output",
			input: &InfoJSONOutput{
				ProjectRoot:  "/tmp/project",
				ProjectName:  "TestProject",
				Collections:  []string{"collections/a.yaml"},
				Environments: []string{"dev"},
				Version:      "0.1.0-dev",
			},
			check: func(t *testing.T, data []byte) {
				if !json.Valid(data) {
					t.Fatalf("not valid JSON: %s", data)
				}
			},
		},
		{
			name: "all_fields_present",
			input: &InfoJSONOutput{
				ProjectRoot:  "/tmp/project",
				ProjectName:  "TestProject",
				Collections:  []string{},
				Environments: []string{},
				Version:      "0.1.0-dev",
			},
			check: func(t *testing.T, data []byte) {
				for _, field := range []string{`"project_root"`, `"project_name"`, `"collections"`, `"environments"`, `"version"`} {
					if !bytes.Contains(data, []byte(field)) {
						t.Errorf("missing field %s in: %s", field, data)
					}
				}
			},
		},
		{
			name: "empty_collections_not_null",
			input: &InfoJSONOutput{
				ProjectRoot:  "/tmp",
				ProjectName:  "Test",
				Collections:  []string{},
				Environments: []string{"dev"},
				Version:      "0.1.0-dev",
			},
			check: func(t *testing.T, data []byte) {
				if bytes.Contains(data, []byte(`"collections": null`)) {
					t.Fatalf("collections should be [] not null, got: %s", data)
				}
				if !bytes.Contains(data, []byte(`"collections": []`)) {
					t.Fatalf("expected empty collections array, got: %s", data)
				}
			},
		},
		{
			name: "empty_environments_not_null",
			input: &InfoJSONOutput{
				ProjectRoot:  "/tmp",
				ProjectName:  "Test",
				Collections:  []string{"collections/a.yaml"},
				Environments: []string{},
				Version:      "0.1.0-dev",
			},
			check: func(t *testing.T, data []byte) {
				if bytes.Contains(data, []byte(`"environments": null`)) {
					t.Fatalf("environments should be [] not null, got: %s", data)
				}
				if !bytes.Contains(data, []byte(`"environments": []`)) {
					t.Fatalf("expected empty environments array, got: %s", data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteInfoJSON(&buf, tc.input); err != nil {
				t.Fatalf("WriteInfoJSON error: %v", err)
			}
			tc.check(t, buf.Bytes())
		})
	}
}

func TestJSONOutputAttemptDetails(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"attempt_details present when retried",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						RetryCount: 1,
						AttemptDetails: []JSONAttemptDetail{
							{Attempt: 1, StatusCode: 503, DurationMs: 20, DelayMs: 0},
							{Attempt: 2, StatusCode: 200, DurationMs: 30, DelayMs: 100},
						},
						Assertions: []JSONAssertion{},
					},
				},
			},
			[]string{`"attempt_details"`, `"attempt": 1`, `"attempt": 2`, `"status_code": 503`},
			nil,
		},
		{
			"attempt_details omitted when no retry",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						Assertions: []JSONAssertion{},
					},
				},
			},
			nil,
			[]string{`"attempt_details"`},
		},
		{
			"attempt_details include error for network failures",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Requests: []JSONRequest{
					{
						Name: "req1", Status: "passed", Method: "GET", URL: "http://example.com",
						RetryCount: 1,
						AttemptDetails: []JSONAttemptDetail{
							{Attempt: 1, DurationMs: 5, DelayMs: 0, Error: "connection refused"},
							{Attempt: 2, StatusCode: 200, DurationMs: 30, DelayMs: 100},
						},
						Assertions: []JSONAssertion{},
					},
				},
			},
			[]string{`"error": "connection refused"`},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			if !json.Valid(data) {
				t.Fatalf("not valid JSON: %s", data)
			}
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestJSONRequest_Warnings_marshalling(t *testing.T) {
	t.Run("includes warnings when present", func(t *testing.T) {
		out := &JSONOutput{
			Name:   "t",
			Status: "passed",
			Requests: []JSONRequest{
				{
					Name:       "a",
					Status:     "passed",
					Assertions: []JSONAssertion{},
					Warnings:   []string{"GraphQL partial success: deprecated field"},
				},
			},
		}
		var buf bytes.Buffer
		if err := WriteJSON(&buf, out); err != nil {
			t.Fatalf("WriteJSON error: %v", err)
		}
		data := buf.Bytes()
		if !bytes.Contains(data, []byte(`"warnings"`)) {
			t.Errorf("expected 'warnings' key in JSON, got: %s", data)
		}
		if !bytes.Contains(data, []byte("deprecated field")) {
			t.Errorf("expected warning message in JSON, got: %s", data)
		}
	})

	t.Run("omits warnings key when empty", func(t *testing.T) {
		out := &JSONOutput{
			Name:   "t",
			Status: "passed",
			Requests: []JSONRequest{
				{
					Name:       "b",
					Status:     "passed",
					Assertions: []JSONAssertion{},
					Warnings:   nil,
				},
			},
		}
		var buf bytes.Buffer
		if err := WriteJSON(&buf, out); err != nil {
			t.Fatalf("WriteJSON error: %v", err)
		}
		data := buf.Bytes()
		if bytes.Contains(data, []byte(`"warnings"`)) {
			t.Errorf("expected 'warnings' key to be absent for empty warnings, got: %s", data)
		}
	})
}

func TestWriteMultiJSON(t *testing.T) {
	out := &MultiJSONOutput{
		Status:     "passed",
		DurationMs: 500,
		Collections: []JSONOutput{
			{Name: "A", Status: "passed", DurationMs: 200, Requests: []JSONRequest{}},
			{Name: "B", Status: "failed", DurationMs: 300, Requests: []JSONRequest{}},
		},
		Total:  2,
		Passed: 1,
		Failed: 1,
	}
	var buf bytes.Buffer
	if err := WriteMultiJSON(&buf, out); err != nil {
		t.Fatalf("WriteMultiJSON error: %v", err)
	}
	data := buf.Bytes()
	if !json.Valid(data) {
		t.Fatalf("output is not valid JSON: %s", data)
	}
	for _, field := range []string{`"status"`, `"collections"`, `"total_collections"`, `"passed_collections"`, `"failed_collections"`} {
		if !bytes.Contains(data, []byte(field)) {
			t.Errorf("missing field %s in: %s", field, data)
		}
	}
	// Decode and verify round-trip.
	var decoded MultiJSONOutput
	if err := json.Unmarshal(bytes.TrimSpace(data), &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Total != 2 {
		t.Errorf("Total = %d, want 2", decoded.Total)
	}
	if decoded.Passed != 1 {
		t.Errorf("Passed = %d, want 1", decoded.Passed)
	}
	if len(decoded.Collections) != 2 {
		t.Errorf("len(Collections) = %d, want 2", len(decoded.Collections))
	}
}

func TestWriteJSON_Summary(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"summary present when set",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Summary:  &SummaryJSON{Total: 4, Passed: 3, Failed: 1, Skipped: 0},
				Requests: []JSONRequest{},
			},
			[]string{`"summary"`, `"total": 4`, `"passed": 3`, `"failed": 1`, `"skipped": 0`},
			nil,
		},
		{
			// Summary field has no omitempty — a nil pointer serialises as
			// "summary": null. buildSummaryJSON guarantees non-nil at all
			// production call sites, but the struct serialisation test
			// documents what happens when the field is left nil directly.
			"summary null when nil pointer set directly",
			&JSONOutput{Name: "Suite", Status: "passed", Requests: []JSONRequest{}},
			[]string{`"summary": null`},
			nil,
		},
		{
			"summary all zeros still emitted",
			&JSONOutput{
				Name: "Suite", Status: "passed",
				Summary:  &SummaryJSON{},
				Requests: []JSONRequest{},
			},
			[]string{`"summary"`, `"total": 0`, `"passed": 0`, `"failed": 0`, `"skipped": 0`},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

func TestWriteJSON_DataDrivenIterations(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"iterations populated for data-driven",
			&JSONOutput{
				Name: "Suite", Status: "passed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{
					Type: "data_driven", Name: "Create User",
					TotalIterations: 2, PassedIterations: 2, FailedIterations: 0,
					Iterations: []DataDrivenIterationJSON{
						{Name: "Create User [1/2]", Status: "passed", DurationMs: 100, DataColumns: map[string]string{"name": "alice"}},
						{Name: "Create User [2/2]", Status: "passed", DurationMs: 110, DataColumns: map[string]string{"name": "bob"}},
					},
				}},
			},
			[]string{`"iterations"`, `"name": "Create User [1/2]"`, `"status": "passed"`, `"duration_ms": 100`, `"data_columns"`, `"name": "alice"`},
			nil,
		},
		{
			"iterations omitted when nil",
			&JSONOutput{
				Name: "Suite", Status: "passed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{Type: "data_driven", Name: "X", TotalIterations: 0}},
			},
			nil,
			[]string{`"iterations"`},
		},
		{
			"iteration without data_columns omits the key",
			&JSONOutput{
				Name: "Suite", Status: "passed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{
					Type: "data_driven", Name: "X", TotalIterations: 1, PassedIterations: 1,
					Iterations: []DataDrivenIterationJSON{{Name: "X [1/1]", Status: "passed", DurationMs: 50}},
				}},
			},
			[]string{`"iterations"`, `"name": "X [1/1]"`},
			[]string{`"data_columns"`},
		},
		{
			"failed iteration status",
			&JSONOutput{
				Name: "Suite", Status: "failed", Requests: []JSONRequest{},
				DataDriven: []DataDrivenJSON{{
					Type: "data_driven", Name: "X", TotalIterations: 1, PassedIterations: 0, FailedIterations: 1,
					Iterations: []DataDrivenIterationJSON{{Name: "X [1/1]", Status: "failed", DurationMs: 33}},
				}},
			},
			[]string{`"status": "failed"`},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}

// TestWriteJSON_SkipReasonPresent verifies that when a JSONRequest carries
// a non-empty SkipReason, the "skip_reason" field appears in the JSON output,
// and when SkipReason is empty it is omitted (omitempty).
func TestWriteJSON_SkipReasonPresent(t *testing.T) {
	tests := []struct {
		name       string
		input      *JSONOutput
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name: "skip_reason present when set",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "skipped-req",
						Status:     "skipped",
						Method:     "GET",
						URL:        "https://example.com",
						SkipReason: "if: false",
						Assertions: []JSONAssertion{},
					},
				},
			},
			wantSubstr: []string{`"skip_reason": "if: false"`},
		},
		{
			name: "skip_reason omitted when empty",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "skipped-req",
						Status:     "skipped",
						Method:     "GET",
						URL:        "https://example.com",
						SkipReason: "",
						Assertions: []JSONAssertion{},
					},
				},
			},
			wantAbsent: []string{`"skip_reason"`},
		},
		{
			name: "skip_reason carries parent-skipped reason",
			input: &JSONOutput{
				Name:   "Suite",
				Status: "passed",
				Requests: []JSONRequest{
					{
						Name:       "notify",
						Status:     "skipped",
						Method:     "POST",
						URL:        "https://example.com",
						SkipReason: "parent skipped: confirm-pending-order",
						Assertions: []JSONAssertion{},
					},
				},
			},
			wantSubstr: []string{`"skip_reason": "parent skipped: confirm-pending-order"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJSON(&buf, tt.input); err != nil {
				t.Fatalf("WriteJSON error: %v", err)
			}
			data := buf.Bytes()
			if !json.Valid(data) {
				t.Fatalf("not valid JSON: %s", data)
			}
			for _, s := range tt.wantSubstr {
				if !bytes.Contains(data, []byte(s)) {
					t.Errorf("output does not contain %q; got: %s", s, data)
				}
			}
			for _, s := range tt.wantAbsent {
				if bytes.Contains(data, []byte(s)) {
					t.Errorf("output should not contain %q; got: %s", s, data)
				}
			}
		})
	}
}
