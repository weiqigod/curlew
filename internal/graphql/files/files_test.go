package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadQuery(t *testing.T) {
	dir := t.TempDir()

	writeFile := func(name, contents string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	writeFile("queries/get_user.graphql", "query GetUser($id: ID!) { user(id: $id) { ...UserFields } }")
	writeFile("fragments/user_fields.graphql", "fragment UserFields on User { id name email }")
	writeFile("fragments/post_fields.graphql", "fragment PostFields on Post { id title author { ...UserFields } }")
	writeFile("fragments/cycle_a.graphql", "fragment A on T { ...B }")
	writeFile("fragments/cycle_b.graphql", "fragment B on T { ...A }")
	writeFile("fragments/self_ref.graphql", "fragment Self on T { id ...Self }")
	writeFile("fragments/dup_a.graphql", "fragment Dup on T { id }")
	writeFile("fragments/dup_b.graphql", "fragment Dup on T { name }")
	writeFile("queries/with_var.graphql", "query { user(id: \"{{user_id}}\") { id } }")
	writeFile("queries/empty.graphql", "")
	// Transitive chain A -> B -> C used to verify topological ordering across
	// more than one level of indirection.
	writeFile("fragments/chain_a.graphql", "fragment A on T { id }")
	writeFile("fragments/chain_b.graphql", "fragment B on T { ...A name }")
	writeFile("fragments/chain_c.graphql", "fragment C on T { ...B label }")
	// Multi-declaration file: only the first declaration's name is recorded.
	writeFile("fragments/multi_decl.graphql", "fragment First on T { id }\nfragment Second on T { name }")
	// Fragment that contains an inline fragment ("... on Type") to verify
	// that the spread extractor does not record "on" as a dependency.
	writeFile("fragments/inline_fragment.graphql", "fragment WithInline on Node { ... on User { id } ...UserFields }")

	// Absolute path for queries/get_user.graphql used by the absolute-path subtest.
	absQueryFile := filepath.Join(dir, "queries/get_user.graphql")

	tests := []struct {
		name       string
		input      LoadQueryInput
		wantErr    error
		wantSubstr []string
		wantOrder  []string
		wantFiles  int
	}{
		{
			name: "inline query no files",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ me { id } }",
			},
			wantSubstr: []string{"{ me { id } }"},
			wantFiles:  0,
		},
		{
			name: "query_file loads from disk",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
			},
			wantSubstr: []string{"query GetUser", "UserFields"},
			wantFiles:  1,
		},
		{
			name: "query_file with single fragment concatenates",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
				Fragments: []string{"fragments/user_fields.graphql"},
			},
			wantSubstr: []string{"fragment UserFields", "query GetUser"},
			wantOrder:  []string{"fragment UserFields", "query GetUser"},
			wantFiles:  2,
		},
		{
			name: "fragment dependency ordered before dependent",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
				Fragments: []string{
					"fragments/post_fields.graphql",
					"fragments/user_fields.graphql",
				},
			},
			wantOrder: []string{"fragment UserFields", "fragment PostFields", "query GetUser"},
			wantFiles: 3,
		},
		{
			name: "transitive fragment chain ordered A before B before C",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...C } }",
				// Deliberately declared in reverse order to force the sort
				// to reorder them.
				Fragments: []string{
					"fragments/chain_c.graphql",
					"fragments/chain_b.graphql",
					"fragments/chain_a.graphql",
				},
			},
			wantOrder: []string{"fragment A", "fragment B", "fragment C", "{ x { ...C } }"},
			wantFiles: 3,
		},
		{
			name: "absolute query_file path is honored",
			input: LoadQueryInput{
				// BaseDir intentionally blank to prove the absolute path is
				// used verbatim.
				BaseDir:   "",
				QueryFile: absQueryFile,
			},
			wantSubstr: []string{"query GetUser"},
			wantFiles:  1,
		},
		{
			name: "multi-declaration fragment file records only first name",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...First } }",
				Fragments:   []string{"fragments/multi_decl.graphql"},
			},
			wantSubstr: []string{"fragment First", "fragment Second"},
			wantFiles:  1,
		},
		{
			name: "inline fragment syntax does not become a dependency",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...WithInline } }",
				Fragments: []string{
					"fragments/inline_fragment.graphql",
					"fragments/user_fields.graphql",
				},
			},
			// UserFields is a real dependency (referenced via ...UserFields),
			// the "... on User" inline fragment must NOT introduce a
			// dependency on a fragment literally named "on".
			wantOrder: []string{"fragment UserFields", "fragment WithInline", "{ x { ...WithInline } }"},
			wantFiles: 2,
		},
		{
			name: "circular fragment dependency returns ErrFragmentCycle",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...A } }",
				Fragments: []string{
					"fragments/cycle_a.graphql",
					"fragments/cycle_b.graphql",
				},
			},
			wantErr: ErrFragmentCycle,
		},
		{
			name: "self-referencing fragment is not a cycle",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...Self } }",
				Fragments:   []string{"fragments/self_ref.graphql"},
			},
			wantSubstr: []string{"fragment Self"},
			wantFiles:  1,
		},
		{
			name: "missing query_file returns ErrQueryFileNotFound",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/missing.graphql",
			},
			wantErr: ErrQueryFileNotFound,
		},
		{
			name: "missing fragment file returns ErrFragmentFileNotFound",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x }",
				Fragments:   []string{"fragments/missing.graphql"},
			},
			wantErr: ErrFragmentFileNotFound,
		},
		{
			name: "both query and query_file returns ErrQueryMutuallyExclusive",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ a }",
				QueryFile:   "queries/get_user.graphql",
			},
			wantErr: ErrQueryMutuallyExclusive,
		},
		{
			name: "no query and no query_file returns ErrMissingQuery",
			input: LoadQueryInput{
				BaseDir: dir,
			},
			wantErr: ErrMissingQuery,
		},
		{
			name: "empty query_file returns ErrMissingQuery",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/empty.graphql",
			},
			wantErr: ErrMissingQuery,
		},
		{
			name: "duplicate fragment name returns error",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x }",
				Fragments: []string{
					"fragments/dup_a.graphql",
					"fragments/dup_b.graphql",
				},
			},
			wantErr: ErrDuplicateFragment,
		},
		{
			name: "variable placeholders preserved in loaded content",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/with_var.graphql",
			},
			wantSubstr: []string{"{{user_id}}"},
			wantFiles:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadQuery(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadQuery() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadQuery() unexpected error: %v", err)
			}
			for _, s := range tt.wantSubstr {
				if !strings.Contains(got.Query, s) {
					t.Errorf("result.Query missing substring %q\nGot:\n%s", s, got.Query)
				}
			}
			if len(tt.wantOrder) > 0 {
				prev := -1
				for _, s := range tt.wantOrder {
					idx := strings.Index(got.Query, s)
					if idx < 0 {
						t.Errorf("result.Query missing expected substring %q\nGot:\n%s", s, got.Query)
						continue
					}
					if idx < prev {
						t.Errorf("substring %q appears before previous; wanted order %v\nGot:\n%s", s, tt.wantOrder, got.Query)
					}
					prev = idx
				}
			}
			if tt.wantFiles > 0 && len(got.FilePaths) != tt.wantFiles {
				t.Errorf("FilePaths count = %d, want %d (%v)", len(got.FilePaths), tt.wantFiles, got.FilePaths)
			}
		})
	}
}

func TestLoadQueryFragmentMissingDeclaration(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.graphql")
	if err := os.WriteFile(p, []byte("{ not a fragment }"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadQuery(LoadQueryInput{
		BaseDir:     dir,
		InlineQuery: "{ x }",
		Fragments:   []string{"bad.graphql"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not contain a 'fragment") {
		t.Fatalf("expected fragment declaration error, got %v", err)
	}
}

func TestExtractFragmentSpreads(t *testing.T) {
	tests := []struct {
		name, body, self string
		want             []string
	}{
		{"no spreads", "fragment X on T { id }", "X", nil},
		{"one spread", "fragment X on T { ...Y }", "X", []string{"Y"}},
		{"multiple spreads deduped", "fragment X on T { ...Y ...Z ...Y }", "X", []string{"Y", "Z"}},
		{"self reference excluded", "fragment X on T { ...X ...Y }", "X", []string{"Y"}},
		{"inline fragment not captured", "fragment X on Node { ... on User { id } ...Y }", "X", []string{"Y"}},
		{"inline fragment only", "fragment X on Node { ... on User { id } }", "X", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFragmentSpreads(tt.body, tt.self)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, g, tt.want[i])
				}
			}
		})
	}
}
