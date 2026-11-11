package runner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/httpexec"
)

// TestRun_JSONBodyKeepsWrittenKeyOrder: a body written as a YAML map is sent
// with its keys in the order written, at every nesting level and after
// interpolation, with and without --parallel.
func TestRun_JSONBodyKeepsWrittenKeyOrder(t *testing.T) {
	var (
		mu  sync.Mutex
		got = map[string]string{}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got[r.URL.Path] = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	csvPath := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(csvPath, []byte("who\nrow-user\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	col := `name: body order
variables:
  base: "` + srv.URL + `"
  who: z-user
requests:
  - name: ordered
    request:
      method: POST
      url: "{{base}}/echo"
      body:
        zeta: 1
        alpha:
          yankee: "{{who}}"
          bravo:
            - delta: 1
              charlie: 2
            - 3
        mike: true
  - name: data driven
    data_driven:
      source: '` + csvPath + `'
    request:
      method: POST
      url: "{{base}}/rows"
      body:
        zulu: "{{who}}"
        able: 1
`
	const want = `{"zeta":1,"alpha":{"yankee":"z-user","bravo":[{"delta":1,"charlie":2},3]},"mike":true}`
	const wantRow = `{"zulu":"row-user","able":1}`
	if _, err := docs.Prose("MANUAL.md", "Curlew keeps the keys of a `body` map in the order you wrote them"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "is sent as JSON with its keys in the order written, at every"); err != nil {
		t.Fatalf("specified claim: %v", err)
	}
	for _, par := range []bool{false, true} {
		got = map[string]string{}
		_, summary, err := Run(context.Background(), parseCollectionYAML(t, col), httpexec.Execute, VarSources{Parallel: par})
		if err != nil {
			t.Fatalf("parallel=%v: Run: %v", par, err)
		}
		if summary.Failed != 0 {
			t.Fatalf("parallel=%v: failed = %d", par, summary.Failed)
		}
		if len(got) != 2 || got["/echo"] != want || got["/rows"] != wantRow {
			t.Errorf("parallel=%v: bodies on the wire = %q, want %q and %q", par, got, want, wantRow)
		}
	}
}
