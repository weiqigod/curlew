package output

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestWriteJUnitXML(t *testing.T) {
	tests := []struct {
		name       string
		input      *JUnitTestSuites
		wantSubstr []string
		wantAbsent []string
	}{
		{
			"valid XML with passing tests",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:     "My Collection",
					Tests:    2,
					Failures: 0,
					Errors:   0,
					Skipped:  0,
					Time:     "1.234",
					TestCases: []JUnitTestCase{
						{Name: "Get Users", ClassName: "My Collection", Time: "0.456"},
						{Name: "Get Items", ClassName: "My Collection", Time: "0.778"},
					},
				}},
			},
			[]string{`<?xml version="1.0"`, `<testsuites>`, `<testsuite`, `<testcase`, `time="`},
			[]string{`<failure`, `<error`, `<skipped`},
		},
		{
			"failure element for assertion failures",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:     "Test",
					Tests:    1,
					Failures: 1,
					TestCases: []JUnitTestCase{{
						Name:      "Check Status",
						ClassName: "Test",
						Time:      "0.100",
						Failure: &JUnitFailure{
							Message: "Expected status 200, got 404",
							Type:    "AssertionFailure",
							Body:    "Expected status 200, got 404",
						},
					}},
				}},
			},
			[]string{`<failure message="`},
			nil,
		},
		{
			"error element for network errors",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:   "Test",
					Tests:  1,
					Errors: 1,
					TestCases: []JUnitTestCase{{
						Name:      "Bad Request",
						ClassName: "Test",
						Error: &JUnitError{
							Message: "connection refused",
							Type:    "ExecutionError",
						},
					}},
				}},
			},
			[]string{`<error message="`},
			nil,
		},
		{
			"skipped element for skipped requests",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:    "Test",
					Tests:   1,
					Skipped: 1,
					TestCases: []JUnitTestCase{{
						Name:      "Skipped Request",
						ClassName: "Test",
						Skipped:   &JUnitSkipped{Message: "dependency failed"},
					}},
				}},
			},
			[]string{`<skipped`},
			nil,
		},
		{
			"time attribute reflects duration in seconds",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:  "Test",
					Tests: 1,
					Time:  "0.500",
					TestCases: []JUnitTestCase{{
						Name:      "Timed",
						ClassName: "Test",
						Time:      "0.123",
					}},
				}},
			},
			[]string{`time="0.123"`},
			nil,
		},
		{
			"empty collection produces valid XML",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:      "Empty",
					Tests:     0,
					TestCases: nil,
				}},
			},
			[]string{`<?xml version="1.0"`, `<testsuites`},
			nil,
		},
		{
			"suite-level counts correct",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:     "Counts",
					Tests:    4,
					Failures: 1,
					Errors:   1,
					Skipped:  1,
					TestCases: []JUnitTestCase{
						{Name: "Pass", ClassName: "Counts", Time: "0.100"},
						{Name: "Fail", ClassName: "Counts", Failure: &JUnitFailure{Message: "fail"}},
						{Name: "Error", ClassName: "Counts", Error: &JUnitError{Message: "err"}},
						{Name: "Skip", ClassName: "Counts", Skipped: &JUnitSkipped{}},
					},
				}},
			},
			[]string{`tests="4"`, `failures="1"`, `errors="1"`, `skipped="1"`},
			nil,
		},
		{
			"classname attribute set to collection name",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:  "My API",
					Tests: 1,
					TestCases: []JUnitTestCase{{
						Name:      "Request",
						ClassName: "My API",
					}},
				}},
			},
			[]string{`classname="My API"`},
			nil,
		},
		{
			"multiple assertion failures concatenated in body",
			&JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:     "Multi",
					Tests:    1,
					Failures: 1,
					TestCases: []JUnitTestCase{{
						Name:      "Multi Fail",
						ClassName: "Multi",
						Failure: &JUnitFailure{
							Message: "Expected status 200, got 404",
							Type:    "AssertionFailure",
							Body:    "Expected status 200, got 404\nExpected body $.id equals 1, got 2",
						},
					}},
				}},
			},
			[]string{`Expected status 200`},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJUnitXML(&buf, tt.input); err != nil {
				t.Fatalf("WriteJUnitXML() error = %v", err)
			}
			got := buf.String()

			for _, sub := range tt.wantSubstr {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing expected substring %q\nGot:\n%s", sub, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output contains unexpected substring %q\nGot:\n%s", absent, got)
				}
			}
		})
	}
}

func TestWriteJUnitXML_roundTrip(t *testing.T) {
	input := &JUnitTestSuites{
		TestSuites: []JUnitTestSuite{{
			Name:     "Round Trip",
			Tests:    2,
			Failures: 1,
			Time:     "1.000",
			TestCases: []JUnitTestCase{
				{Name: "Pass", ClassName: "Round Trip", Time: "0.500"},
				{Name: "Fail", ClassName: "Round Trip", Time: "0.500", Failure: &JUnitFailure{Message: "bad", Type: "AssertionFailure", Body: "bad result"}},
			},
		}},
	}

	var buf bytes.Buffer
	if err := WriteJUnitXML(&buf, input); err != nil {
		t.Fatalf("WriteJUnitXML() error = %v", err)
	}

	// Verify XML is well-formed by unmarshaling
	var parsed JUnitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v\nXML:\n%s", err, buf.String())
	}

	if len(parsed.TestSuites) != 1 {
		t.Fatalf("expected 1 testsuite, got %d", len(parsed.TestSuites))
	}
	suite := parsed.TestSuites[0]
	if suite.Name != "Round Trip" {
		t.Errorf("suite Name = %q, want %q", suite.Name, "Round Trip")
	}
	if suite.Tests != 2 {
		t.Errorf("suite Tests = %d, want 2", suite.Tests)
	}
	if suite.Failures != 1 {
		t.Errorf("suite Failures = %d, want 1", suite.Failures)
	}
	if len(suite.TestCases) != 2 {
		t.Fatalf("expected 2 testcases, got %d", len(suite.TestCases))
	}
	if suite.TestCases[1].Failure == nil {
		t.Error("expected failure element on second testcase")
	}
}

func TestWriteJUnitXML_specialChars(t *testing.T) {
	input := &JUnitTestSuites{
		TestSuites: []JUnitTestSuite{{
			Name:  "Special <Chars> & \"Quotes\"",
			Tests: 1,
			TestCases: []JUnitTestCase{{
				Name:      "Test <with> & \"special\" chars",
				ClassName: "Special <Chars> & \"Quotes\"",
			}},
		}},
	}

	var buf bytes.Buffer
	if err := WriteJUnitXML(&buf, input); err != nil {
		t.Fatalf("WriteJUnitXML() error = %v", err)
	}

	// Verify the XML is still well-formed (special chars must be escaped)
	var parsed JUnitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v\nXML:\n%s", err, buf.String())
	}
	if parsed.TestSuites[0].Name != "Special <Chars> & \"Quotes\"" {
		t.Errorf("name not round-tripped: got %q", parsed.TestSuites[0].Name)
	}
}

// TestWriteJUnitXML_SkipReasonInMessage verifies that when JUnitSkipped.Message
// is set to a skip reason (e.g. "if: false"), the generated XML includes the
// message= attribute on the <skipped> element.
func TestWriteJUnitXML_SkipReasonInMessage(t *testing.T) {
	tests := []struct {
		name       string
		input      *JUnitTestSuites
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name: "if-false reason appears in skipped message attribute",
			input: &JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:    "Suite",
					Tests:   1,
					Skipped: 1,
					TestCases: []JUnitTestCase{{
						Name:      "confirm-pending-order",
						ClassName: "Suite",
						Skipped:   &JUnitSkipped{Message: "if: false"},
					}},
				}},
			},
			wantSubstr: []string{`<skipped`, `message="if: false"`},
		},
		{
			name: "parent-skipped reason appears in skipped message attribute",
			input: &JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:    "Suite",
					Tests:   1,
					Skipped: 1,
					TestCases: []JUnitTestCase{{
						Name:      "notify",
						ClassName: "Suite",
						Skipped:   &JUnitSkipped{Message: "parent skipped: confirm-pending-order"},
					}},
				}},
			},
			wantSubstr: []string{`<skipped`, `message="parent skipped: confirm-pending-order"`},
		},
		{
			name: "empty message still produces skipped element",
			input: &JUnitTestSuites{
				TestSuites: []JUnitTestSuite{{
					Name:    "Suite",
					Tests:   1,
					Skipped: 1,
					TestCases: []JUnitTestCase{{
						Name:      "req",
						ClassName: "Suite",
						Skipped:   &JUnitSkipped{},
					}},
				}},
			},
			wantSubstr: []string{`<skipped`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteJUnitXML(&buf, tt.input); err != nil {
				t.Fatalf("WriteJUnitXML error: %v", err)
			}
			out := buf.String()
			// Verify the XML round-trips cleanly.
			var parsed JUnitTestSuites
			if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
				t.Fatalf("xml.Unmarshal error: %v\nXML:\n%s", err, out)
			}
			for _, s := range tt.wantSubstr {
				if !strings.Contains(out, s) {
					t.Errorf("output does not contain %q; got:\n%s", s, out)
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(out, s) {
					t.Errorf("output should not contain %q; got:\n%s", s, out)
				}
			}
		})
	}
}

// TestWriteJUnitXML_CelFailureMessagePreserved verifies that a CEL assertion
// failure message — which is a multiline string containing the expression
// source and resolved sub-values — is preserved verbatim in the <failure>
// body element. This locks in that the JUnit formatter does not truncate or
// escape the newline-separated reference lines.
func TestWriteJUnitXML_CelFailureMessagePreserved(t *testing.T) {
	celMsg := "response.body.total == response.body.items.map(i, i.price).sum()\n" +
		"  response.body.total = 9.5\n" +
		"  response.body.items = [{price:5} {price:5.5}]"

	input := &JUnitTestSuites{
		TestSuites: []JUnitTestSuite{{
			Name:     "CEL Suite",
			Tests:    1,
			Failures: 1,
			TestCases: []JUnitTestCase{{
				Name:      "cel-fails",
				ClassName: "CEL Suite",
				Failure: &JUnitFailure{
					Message: "cel assertion failed",
					Type:    "AssertionFailure",
					Body:    celMsg,
				},
			}},
		}},
	}

	var buf bytes.Buffer
	if err := WriteJUnitXML(&buf, input); err != nil {
		t.Fatalf("WriteJUnitXML: %v", err)
	}
	out := buf.String()

	// The failure body must contain the expression source.
	if !strings.Contains(out, "response.body.total == response.body.items") {
		t.Errorf("JUnit output missing CEL expression source:\n%s", out)
	}
	// The resolved sub-values must be present.
	if !strings.Contains(out, "response.body.total = 9.5") {
		t.Errorf("JUnit output missing resolved sub-value line:\n%s", out)
	}
	// Round-trip: the XML must still parse cleanly with the multiline body.
	var parsed JUnitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("xml.Unmarshal: %v\nXML:\n%s", err, out)
	}
}
