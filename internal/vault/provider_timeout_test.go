package vault

import (
	"context"
	"testing"
	"time"
)

func TestProviderCommandsWaitForInteractiveApproval(t *testing.T) {
	var gotTimeout time.Duration
	var gotProgram string
	original := runProgram
	runProgram = func(_ context.Context, timeout time.Duration, program string, _, _ []string) (string, error) {
		gotTimeout, gotProgram = timeout, program
		return "value", nil
	}
	t.Cleanup(func() { runProgram = original })

	if _, err := NewOnePasswordProvider(nil).Fetch(context.Background(), "op://V/item/field"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotProgram != "op" {
		t.Fatalf("program = %q, want op", gotProgram)
	}
	if gotTimeout < 2*time.Minute {
		t.Fatalf("provider timeout = %v; a 1Password approval prompt needs at least 2 minutes", gotTimeout)
	}
}
