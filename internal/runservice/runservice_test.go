package runservice_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/config"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/output/events"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/runservice"
	"github.com/peterlindqvist/apitest/internal/variable"
)

// writeProject scaffolds a minimal apitest project and returns the absolute
// collection path.
func writeProject(t *testing.T, collectionYAML string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apitest.yaml"), []byte("project_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	colDir := filepath.Join(root, "collections")
	if err := os.MkdirAll(colDir, 0o755); err != nil {
		t.Fatal(err)
	}
	colPath := filepath.Join(colDir, "c.yaml")
	if err := os.WriteFile(colPath, []byte(collectionYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return colPath
}

func okExec(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
	return &httpexec.Result{
		StatusCode: 200,
		Body:       []byte(`{"token":"s3cret-value","ok":true}`),
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestExecute_HappyPath(t *testing.T) {
	colPath := writeProject(t, `name: T
requests:
  - name: Ping
    request:
      method: GET
      url: "http://x.test/ping"
    assertions:
      status: 200
`)
	res, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		RunID:          runner.NewRunID(),
	}, okExec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Summary == nil || res.Summary.Total != 1 || res.Summary.Passed != 1 {
		t.Fatalf("summary = %+v, want 1 total 1 passed", res.Summary)
	}
	if res.Collection == nil || res.Collection.Name != "T" {
		t.Errorf("collection not returned")
	}
	if res.PreSensitive == nil || res.Sensitive == nil {
		t.Errorf("sensitive sets not built")
	}
}

func TestExecute_ParseError_WrappedAsCollectionInvalid(t *testing.T) {
	colPath := writeProject(t, "name: [broken\n")
	_, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		RunID:          runner.NewRunID(),
	}, okExec)
	if !errors.Is(err, runservice.ErrCollectionInvalid) {
		t.Fatalf("err = %v, want ErrCollectionInvalid", err)
	}
}

func TestExecute_EnvNotFound_PassesThrough(t *testing.T) {
	colPath := writeProject(t, `name: T
requests:
  - name: Ping
    request: {method: GET, url: "http://x.test"}
`)
	_, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		EnvName:        "nope",
		RunID:          runner.NewRunID(),
	}, okExec)
	if !errors.Is(err, config.ErrEnvironmentNotFound) {
		t.Fatalf("err = %v, want ErrEnvironmentNotFound", err)
	}
}

func TestExecute_UsesEnvironmentLocale(t *testing.T) {
	colPath := writeProject(t, `name: T
requests:
  - name: Ping
    request: {method: GET, url: "http://x.test/{{$faker.fullName}}"}
`)
	root := filepath.Dir(filepath.Dir(colPath))
	envDir := filepath.Join(root, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(`variables: {}
config:
  locale: xx-YY
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		EnvName:        "dev",
		RunID:          runner.NewRunID(),
	}, okExec)
	if !errors.Is(err, variable.ErrLocaleUnknown) {
		t.Fatalf("Execute error = %v; want environment config locale error", err)
	}
}

func TestExecute_SelectionNoMatch_PassesThroughSentinel(t *testing.T) {
	colPath := writeProject(t, `name: T
requests:
  - name: Ping
    request: {method: GET, url: "http://x.test"}
`)
	_, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		Selection:      []string{"Does Not Exist"},
		RunID:          runner.NewRunID(),
	}, okExec)
	if !errors.Is(err, runner.ErrNoMatchingRequests) {
		t.Fatalf("err = %v, want ErrNoMatchingRequests", err)
	}
}

func TestExecute_EmitterSinkRedactsWithPreRunSet(t *testing.T) {
	// A sensitive collection variable's value must be redacted from event
	// bodies via the pre-run set Execute wires into the sink.
	colPath := writeProject(t, `name: T
variables:
  api_token:
    value: "s3cret-value"
    sensitive: true
requests:
  - name: Ping
    request:
      method: GET
      url: "http://x.test/ping"
`)
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{ApitestVersion: "test", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	sink := runservice.NewEmitterSink(em, &bytes.Buffer{}, nil, false)
	_, err = runservice.Execute(context.Background(), runservice.Request{
		CollectionPath: colPath,
		RunID:          "r1",
		Sink:           sink,
	}, okExec)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "s3cret-value") {
		t.Errorf("sensitive value leaked into event stream:\n%s", out)
	}
	if !strings.Contains(out, variable.Redacted) {
		t.Errorf("expected %q marker in event stream:\n%s", variable.Redacted, out)
	}
}

func TestExecute_RequestIDPrefix(t *testing.T) {
	colPath := writeProject(t, `name: T
requests:
  - name: Ping
    request: {method: GET, url: "http://x.test"}
`)
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{ApitestVersion: "test", RunID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	sink := runservice.NewEmitterSink(em, &bytes.Buffer{}, nil, false)
	if _, err := runservice.Execute(context.Background(), runservice.Request{
		CollectionPath:  colPath,
		RunID:           "r1",
		RequestIDPrefix: "c3-",
		Sink:            sink,
	}, okExec); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(buf.String(), `"request_id":"c3-req-1"`) {
		t.Errorf("prefixed request id missing from stream:\n%s", buf.String())
	}
}

func TestBuildPostRunSensitive_IncludesRuntimeSets(t *testing.T) {
	rt := variable.NewSensitiveSet()
	rt.Add("runtime_secret")
	rt.AddValue("rv")
	in := runservice.SensitiveInputs{}
	s := runservice.BuildPostRunSensitive(in, &runner.Summary{RuntimeSensitive: rt})
	if !s.IsSensitive("runtime_secret") {
		t.Error("post-run set missing RuntimeSensitive names")
	}
	pre := runservice.BuildPreRunSensitive(in)
	if pre.IsSensitive("runtime_secret") {
		t.Error("pre-run set unexpectedly contains runtime names")
	}
}
