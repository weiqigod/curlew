package datadriven

import (
	"errors"
	"testing"
)

func TestEvalFilter(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		row     Row
		want    bool
		wantErr error
	}{
		// Comparison operators
		{"equals string", "{{name}} == 'john'", Row{"name": "john"}, true, nil},
		{"equals string false", "{{name}} == 'jane'", Row{"name": "john"}, false, nil},
		{"equals double quotes", `{{name}} == "john"`, Row{"name": "john"}, true, nil},
		{"not equals", "{{name}} != 'john'", Row{"name": "jane"}, true, nil},
		{"not equals false", "{{name}} != 'john'", Row{"name": "john"}, false, nil},
		{"greater than numeric", "{{age}} > 18", Row{"age": "25"}, true, nil},
		{"greater than numeric false", "{{age}} > 18", Row{"age": "15"}, false, nil},
		{"greater than or equal", "{{age}} >= 18", Row{"age": "18"}, true, nil},
		{"greater than or equal false", "{{age}} >= 18", Row{"age": "17"}, false, nil},
		{"less than", "{{age}} < 18", Row{"age": "15"}, true, nil},
		{"less than false", "{{age}} < 18", Row{"age": "25"}, false, nil},
		{"less than or equal", "{{age}} <= 18", Row{"age": "18"}, true, nil},
		{"less than or equal false", "{{age}} <= 18", Row{"age": "19"}, false, nil},

		// String operators
		{"contains true", "{{email}} contains 'example'", Row{"email": "a@example.com"}, true, nil},
		{"contains false", "{{email}} contains 'test'", Row{"email": "a@example.com"}, false, nil},
		{"starts_with true", "{{name}} starts_with 'jo'", Row{"name": "john"}, true, nil},
		{"starts_with false", "{{name}} starts_with 'ja'", Row{"name": "john"}, false, nil},
		{"ends_with true", "{{email}} ends_with '.com'", Row{"email": "a@example.com"}, true, nil},
		{"ends_with false", "{{email}} ends_with '.org'", Row{"email": "a@example.com"}, false, nil},

		// Boolean logic
		{"AND both true", "{{age}} >= 18 AND {{country}} == 'US'", Row{"age": "25", "country": "US"}, true, nil},
		{"AND one false", "{{age}} >= 18 AND {{country}} == 'US'", Row{"age": "25", "country": "UK"}, false, nil},
		{"AND both false", "{{age}} >= 18 AND {{country}} == 'US'", Row{"age": "15", "country": "UK"}, false, nil},
		{"OR one true", "{{age}} >= 18 OR {{country}} == 'US'", Row{"age": "15", "country": "US"}, true, nil},
		{"OR both true", "{{age}} >= 18 OR {{country}} == 'US'", Row{"age": "25", "country": "US"}, true, nil},
		{"OR both false", "{{age}} >= 18 OR {{country}} == 'US'", Row{"age": "15", "country": "UK"}, false, nil},
		{"NOT true", "NOT {{active}} == 'false'", Row{"active": "true"}, true, nil},
		{"NOT false", "NOT {{active}} == 'true'", Row{"active": "true"}, false, nil},

		// Complex boolean expressions
		{"AND then OR", "{{age}} >= 18 AND {{country}} == 'US' OR {{vip}} == 'true'", Row{"age": "15", "country": "UK", "vip": "true"}, true, nil},

		// Edge cases
		{"empty expression", "", Row{"a": "1"}, true, nil},
		{"whitespace only", "   ", Row{"a": "1"}, true, nil},
		{"missing variable", "{{missing}} == 'x'", Row{}, false, nil},
		{"numeric string comparison", "{{val}} > 5", Row{"val": "10"}, true, nil},
		{"float comparison", "{{price}} >= 9.99", Row{"price": "10.50"}, true, nil},
		{"float equality", "{{price}} == 3.14", Row{"price": "3.14"}, true, nil},
		{"unquoted string comparison", "{{name}} == john", Row{"name": "john"}, true, nil},

		// Error cases
		{"invalid operator", "{{age}} LIKE 'john'", Row{"age": "25"}, false, ErrInvalidFilter},
		{"unclosed variable", "{{age >= 18", Row{"age": "25"}, false, ErrInvalidFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvalFilter(tt.expr, tt.row)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("EvalFilter(%q) = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}
