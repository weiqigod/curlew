package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §11.5's matrix: five rows crossing a producer's outcome with whether the
// dependent supplied a default, each stating whether the dependent runs.
//
// Reading it is what turned up the `|default:` bug — the second row promises
// the dependent runs "with the default", and it ran with the literal
// placeholder in its URL. The other four rows had never been checked either,
// and the two Failure rows carry the safety property: a dependent must be
// skipped when its producer failed *even if it has a default*, because a
// default is not a substitute for a resource that was never created.

// skipScenario builds a collection matching one row of the matrix.
type skipScenario struct {
	producerFails bool
	extracts      bool
	hasDefault    bool
}

func (s skipScenario) collection(t *testing.T, dir, okURL, failURL string) string {
	t.Helper()

	producerURL := okURL
	if s.producerFails {
		producerURL = failURL
	}
	// A path that resolves only when the body carries it, so "extracted: No"
	// is a real extraction miss rather than a different request.
	jsonPath := "$.user_id"
	if !s.extracts {
		jsonPath = "$.not_in_the_body"
	}
	reference := "{{user_id}}"
	if s.hasDefault {
		reference = "{{user_id|default:DEFAULTED}}"
	}

	body := fmt.Sprintf(
		"name: skip-matrix\nrequests:\n"+
			"  - name: producer\n    request:\n      method: GET\n      url: %q\n"+
			"    extract:\n      user_id: %q\n"+
			"  - name: dependent\n    request:\n      method: GET\n      url: \"%s/%s\"\n",
		producerURL, jsonPath, okURL, reference)

	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

// runOutcome is what the JSON report says happened to the dependent.
type runOutcome struct {
	status string
	url    string
}

func runSkipScenario(t *testing.T, bin, collection string) runOutcome {
	t.Helper()

	cmd := exec.Command(bin, "run", collection, "--format", "json")
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, _ := cmd.Output()

	var doc struct {
		Requests []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			URL    string `json:"url"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("parsing the run report: %v\n%s", err, out)
	}
	for _, r := range doc.Requests {
		if r.Name == "dependent" {
			return runOutcome{status: r.Status, url: r.URL}
		}
	}
	// No row for the dependent at all: the run ended before it was reached.
	// That is neither "runs" nor "skipped", and the caller has to say so with
	// the scenario in hand.
	return runOutcome{status: "absent", url: strings.TrimSpace(string(out))}
}

func TestDocTables_theSkipMatrixIsWhatHappens(t *testing.T) {
	bin := buildBinary(t)

	// A local server: one route with a body to extract from, one that fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fail") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"EXTRACTED"}`))
	}))
	defer srv.Close()

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md",
		"Producer outcome", "Variable extracted", "Dependent has default", "Dependent runs")
	if err != nil {
		t.Fatalf("skip matrix: %v", err)
	}
	cols := map[string]int{}
	for _, name := range []string{"Producer outcome", "Variable extracted", "Dependent has default", "Dependent runs"} {
		cols[name] = docs.Column(hdr, name)
		if cols[name] < 0 {
			t.Fatalf("skip matrix lost the %q column: %v", name, hdr)
		}
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= cols["Dependent runs"] {
			continue
		}
		outcome := row[cols["Producer outcome"]]
		extracted := row[cols["Variable extracted"]]
		hasDefault := row[cols["Dependent has default"]]
		expected := row[cols["Dependent runs"]]

		scenario := skipScenario{
			producerFails: strings.EqualFold(outcome, "Failure"),
			extracts:      strings.EqualFold(extracted, "Yes"),
			// "—" means the column does not apply; run it without a default.
			hasDefault: strings.EqualFold(hasDefault, "Yes"),
		}
		checked++

		got := runSkipScenario(t, bin, scenario.collection(t, t.TempDir(), srv.URL, srv.URL+"/fail"))
		label := fmt.Sprintf("producer=%s extracted=%s default=%s", outcome, extracted, hasDefault)

		wantsSkip := strings.EqualFold(strings.TrimSpace(expected), "Skipped")
		if got.status == "absent" {
			t.Errorf("§11.5 [%s] says %q; the dependent does not appear in the run at all — "+
				"the run ended before it:\n%s", label, expected, got.url)
			continue
		}
		if wantsSkip {
			if got.status != "skipped" {
				t.Errorf("§11.5 [%s] says %q; the dependent was %q (url %s)",
					label, expected, got.status, got.url)
			}
			continue
		}

		if got.status == "skipped" {
			t.Errorf("§11.5 [%s] says %q; the dependent was skipped", label, expected)
			continue
		}
		// It ran — with what the row says it should have run with.
		if strings.Contains(expected, "extracted value") && !strings.Contains(got.url, "EXTRACTED") {
			t.Errorf("§11.5 [%s] says %q; the URL was %s", label, expected, got.url)
		}
		if strings.Contains(expected, "the default") && !strings.Contains(got.url, "DEFAULTED") {
			t.Errorf("§11.5 [%s] says %q; the URL was %s", label, expected, got.url)
		}
		// Whatever the row says, a placeholder must never reach the wire.
		if strings.Contains(got.url, "{{") {
			t.Errorf("§11.5 [%s]: the dependent sent an uninterpolated reference: %s", label, got.url)
		}
	}

	if checked == 0 {
		t.Fatal("no matrix rows executed; the table moved or its header changed")
	}
	t.Logf("%d matrix rows executed", checked)
}
