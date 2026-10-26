package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/vault"
)

type opRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (o *opRecorder) exec(_ context.Context, command string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	path := strings.Trim(strings.TrimPrefix(command, "op read "), `'"`)
	o.paths = append(o.paths, path)
	parts := strings.Split(path, "/")
	return "secret-" + parts[len(parts)-2], nil
}

func (o *opRecorder) fetched() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.paths...)
}

type memorySecretStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memorySecretStore) Load(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[key]
	return v, ok
}

func (m *memorySecretStore) Save(key, value string, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
}

func opSecrets(ttl int) *vault.SecretsConfig {
	return &vault.SecretsConfig{
		Provider: vault.Provider1Password,
		Keys: map[string]string{
			"dev_token":  "op://V/dev/password",
			"test_token": "op://V/test/password",
			"prod_token": "op://V/prod/password",
			"cel_secret": "op://V/cel/password",
		},
		CacheTTL: ttl,
	}
}

func TestRun_FetchesOnlySecretsTheCollectionUses(t *testing.T) {
	op := &opRecorder{}
	col := &parser.Collection{
		Name: "only used secrets",
		Requests: parser.Section{Items: []parser.RequestItem{{
			Name:    "Get",
			If:      `vars.cel_secret != ""`,
			Request: parser.Request{Method: "GET", URL: "https://example.test/x", Headers: map[string]string{"Authorization": "Bearer {{token}}"}},
		}}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{
		EnvFile:       map[string]string{"token": "{{dev_token}}"},
		Secrets:       opSecrets(0),
		VaultExecutor: op.exec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.Join(op.fetched(), ",")
	if !strings.Contains(got, "op://V/dev/password") || !strings.Contains(got, "op://V/cel/password") || len(op.fetched()) != 2 {
		t.Fatalf("fetched %q, want only the dev token (through the env variable) and the CEL secret", got)
	}
	if len(results) != 1 || results[0].Skipped {
		t.Fatalf("results = %+v, want the request to run", results)
	}
}

func TestRun_DoesNotFetchSecretsOverriddenByVar(t *testing.T) {
	op := &opRecorder{}
	col := &parser.Collection{
		Name: "override",
		Requests: parser.Section{Items: []parser.RequestItem{{
			Name:    "Get",
			Request: parser.Request{Method: "GET", URL: "https://example.test/{{dev_token}}"},
		}}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{
		CLI:           map[string]string{"dev_token": "from-cli"},
		Secrets:       opSecrets(0),
		VaultExecutor: op.exec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.fetched()) != 0 {
		t.Fatalf("fetched %v, want nothing: --var overrides the secret", op.fetched())
	}
	if results[0].URL != "https://example.test/from-cli" {
		t.Fatalf("URL = %q", results[0].URL)
	}
}

func TestRun_FindsSecretsInBodyFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "body.json"), []byte(`{"token":"{{test_token}}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	colPath := filepath.Join(dir, "c.yaml")
	yaml := "name: body file\nrequests:\n  - name: Post\n    request:\n      method: POST\n      url: https://example.test/x\n      body_file: body.json\n"
	if err := os.WriteFile(colPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(colPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	op := &opRecorder{}
	var sent string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		switch b := req.Body.(type) {
		case string:
			sent = b
		case []byte:
			sent = string(b)
		default:
			sent = fmt.Sprintf("%v", b)
		}
		return successExecutor(context.Background(), req)
	}
	if _, _, err := Run(context.Background(), col, exec, VarSources{Secrets: opSecrets(0), VaultExecutor: op.exec, CollectionDir: dir}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := op.fetched(); len(got) != 1 || got[0] != "op://V/test/password" {
		t.Fatalf("fetched %v, want only the test token used in body.json", got)
	}
	if !strings.Contains(sent, "secret-test") {
		t.Fatalf("body sent = %q, want the resolved secret", sent)
	}
}

func TestRun_SecretCacheTTLServesLaterRuns(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "Later runs, including runs from `curlew ui`, reuse them until they expire"); err != nil {
		t.Fatal(err)
	}
	store := &memorySecretStore{values: map[string]string{}}
	col := &parser.Collection{
		Name: "cached",
		Requests: parser.Section{Items: []parser.RequestItem{{
			Name:    "Get",
			Request: parser.Request{Method: "GET", URL: "https://example.test/{{dev_token}}"},
		}}},
	}
	for run := 1; run <= 2; run++ {
		op := &opRecorder{}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       opSecrets(3600),
			VaultExecutor: op.exec,
			SecretStore:   store,
		})
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if want := map[int]int{1: 1, 2: 0}[run]; len(op.fetched()) != want {
			t.Fatalf("run %d fetched %v, want %d calls", run, op.fetched(), want)
		}
		if results[0].URL != "https://example.test/secret-dev" {
			t.Fatalf("run %d URL = %q", run, results[0].URL)
		}
	}
}

func TestRun_SkippedRequestDoesNotFetchItsSecret(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "A request skipped by its `if:` guard or left out by `--only` never triggers a fetch"); err != nil {
		t.Fatal(err)
	}
	op := &opRecorder{}
	col := &parser.Collection{
		Name: "guarded prod",
		Requests: parser.Section{Items: []parser.RequestItem{{
			Name:    "Prod search",
			If:      `vars.allow_production == "true"`,
			Request: parser.Request{Method: "GET", URL: "https://example.test/x", Headers: map[string]string{"Authorization": "Bearer {{token}}"}},
		}}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{
		EnvFile:       map[string]string{"token": "{{prod_token}}", "allow_production": "false"},
		Secrets:       opSecrets(0),
		VaultExecutor: op.exec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.fetched()) != 0 {
		t.Fatalf("fetched %v for a request the if: guard skips", op.fetched())
	}
	if len(results) != 1 || !results[0].Skipped {
		t.Fatalf("results = %+v, want the request skipped", results)
	}
}

func TestRun_OnlySelectedRequestsFetchSecrets(t *testing.T) {
	op := &opRecorder{}
	col := &parser.Collection{
		Name: "selection",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Dev", Request: parser.Request{Method: "GET", URL: "https://example.test/{{dev_token}}"}},
			{Name: "Test", Request: parser.Request{Method: "GET", URL: "https://example.test/{{test_token}}"}},
		}},
	}
	if _, _, err := Run(context.Background(), col, successExecutor, VarSources{
		Selection:     []string{"Dev"},
		Secrets:       opSecrets(0),
		VaultExecutor: op.exec,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := op.fetched(); len(got) != 1 || got[0] != "op://V/dev/password" {
		t.Fatalf("fetched %v, want only the selected request's secret", got)
	}
}

func TestRun_SecretIsFetchedWhenAGuardOpensDuringTheRun(t *testing.T) {
	op := &opRecorder{}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond, Body: []byte(`{"allow":"true"}`)}, nil
	}
	var sentURL string
	wrapped := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		sentURL = req.URL
		return exec(ctx, req)
	}
	col := &parser.Collection{
		Name: "guard opens",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Ask", Request: parser.Request{Method: "GET", URL: "https://example.test/ask"}, Extract: map[string]string{"allow": "$.allow"}},
			{Name: "Use", If: `vars.allow == "true"`, Request: parser.Request{Method: "GET", URL: "https://example.test/{{token}}"}},
		}},
	}
	results, _, err := Run(context.Background(), col, wrapped, VarSources{
		EnvFile:       map[string]string{"token": "{{prod_token}}", "allow": "false"},
		Secrets:       opSecrets(0),
		VaultExecutor: op.exec,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 || results[1].Skipped {
		t.Fatalf("results = %+v, want the second request to run", results)
	}
	if sentURL != "https://example.test/secret-prod" {
		t.Fatalf("sent %q, want the secret fetched on use", sentURL)
	}
	if got := op.fetched(); len(got) != 1 {
		t.Fatalf("fetched %v, want exactly one fetch", got)
	}
}
