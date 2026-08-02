package parser

import (
	"errors"
	"testing"
)

func TestSlug_Derive(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// ASCII basics
		{"simple ASCII lowercase", "get-user", "get-user", false},
		{"ASCII with spaces", "Get user", "get-user", false},
		{"ASCII with mixed case", "GET User", "get-user", false},
		{"single word lowercase", "users", "users", false},
		{"single word uppercase", "USERS", "users", false},

		// Punctuation
		{"trailing punctuation", "Create Post!", "create-post", false},
		{"leading punctuation", "!Create", "create", false},
		{"interior punctuation", "create.post", "create-post", false},
		{"quotes and apostrophes", "Bob's request", "bob-s-request", false},
		{"runs of punctuation collapsed", "a!!!b", "a-b", false},
		{"every punctuation char", "a!@#$%^&*()_+={}b", "a-b", false},

		// Digits
		{"digits preserved", "request 42", "request-42", false},
		{"digit-only name", "200", "200", false},
		{"digits at start", "404 not found", "404-not-found", false},

		// Whitespace edges
		{"leading whitespace", "   leading", "leading", false},
		{"trailing whitespace", "trailing   ", "trailing", false},
		{"interior whitespace runs", "hello    world", "hello-world", false},
		{"mixed whitespace types", "tab\there", "tab-here", false},

		// Unicode
		{"accented characters", "Héllo", "hello", false},
		{"unicode strips to ASCII", "Café", "cafe", false},
		{"combining marks", "naïve", "naive", false},
		{"mixed unicode and ascii", "Héllo 世界", "hello", false},
		{"emoji separator", "ship 🚢 it", "ship-it", false},
		{"all-CJK falls through to empty", "世界", "", true},

		// Empty / reject
		{"empty string rejected", "", "", true},
		{"whitespace-only rejected", "    ", "", true},
		{"tabs only rejected", "\t\t", "", true},
		{"punctuation-only rejected", "!!!", "", true},
		{"hyphens-only rejected", "---", "", true},

		// Hyphen handling
		{"hyphen-separated kept", "a-b-c", "a-b-c", false},
		{"runs of hyphens collapsed", "a---b", "a-b", false},
		{"trailing hyphens trimmed", "abc---", "abc", false},
		{"leading hyphens trimmed", "---abc", "abc", false},

		// Realistic request names
		{"GET pattern", "GET /users/{id}", "get-users-id", false},
		{"data-driven iter", "Create user [1/3]", "create-user-1-3", false},
		{"GraphQL query", "query GetUser { user { id } }", "query-getuser-user-id", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Slug(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Slug(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrSlugEmpty) {
				t.Errorf("Slug(%q) error = %v, expected wraps ErrSlugEmpty", tt.input, err)
			}
		})
	}
}
