package errors

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"testing"
)

// testSentinel is a dedicated sentinel used only by this test file. It does not
// pollute the shared registry because we register it lazily at the top of each
// test that needs it, and the registry is process-global. Using a unique error
// value per test avoids cross-test interference.
var (
	errTestParse    = stderrors.New("test: parse failure")
	errTestAuth     = stderrors.New("test: auth failure")
	errTestUnknown  = stderrors.New("test: unknown failure")
	errTestAssert   = stderrors.New("test: assertion failure")
	errTestOverride = stderrors.New("test: override failure")
)

func init() {
	RegisterPackage("classify_test",
		RegisteredError{
			Name: "errTestParse",
			Err:  errTestParse,
			Hint: ClassifiedHint{Category: CategoryParse, Code: "TEST_PARSE", Hint: "Fix the YAML"},
		},
		RegisteredError{
			Name: "errTestAuth",
			Err:  errTestAuth,
			Hint: ClassifiedHint{Category: CategoryAuth, Code: "TEST_AUTH", Hint: "Refresh the token"},
		},
		RegisteredError{
			Name: "errTestAssert",
			Err:  errTestAssert,
			Hint: ClassifiedHint{Category: CategoryAssertion, Code: "TEST_ASSERT", Hint: "Check expected vs actual"},
		},
		RegisteredError{
			Name: "errTestOverride",
			Err:  errTestOverride,
			Hint: ClassifiedHint{Category: CategoryParse, Code: "TEST_OVERRIDE", Hint: "Registered hint"},
		},
	)
}

func TestClassifyError_Nil(t *testing.T) {
	t.Parallel()
	if got := ClassifyError(nil); got != nil {
		t.Fatalf("ClassifyError(nil) = %+v, want nil", got)
	}
}

func TestClassifyError_RegisteredSentinel(t *testing.T) {
	t.Parallel()
	s := ClassifyError(errTestParse)
	if s == nil {
		t.Fatal("ClassifyError returned nil for known sentinel")
	}
	if s.Category != CategoryParse {
		t.Errorf("Category = %q, want %q", s.Category, CategoryParse)
	}
	if s.Code != "TEST_PARSE" {
		t.Errorf("Code = %q, want TEST_PARSE", s.Code)
	}
	if s.Hint != "Fix the YAML" {
		t.Errorf("Hint = %q, want 'Fix the YAML'", s.Hint)
	}
	if s.Message != errTestParse.Error() {
		t.Errorf("Message = %q, want %q", s.Message, errTestParse.Error())
	}
}

func TestClassifyError_WrappedSentinel(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("context: %w", errTestAuth)
	s := ClassifyError(wrapped)
	if s == nil {
		t.Fatal("ClassifyError returned nil for wrapped sentinel")
	}
	if s.Category != CategoryAuth {
		t.Errorf("Category = %q, want %q", s.Category, CategoryAuth)
	}
	if s.Code != "TEST_AUTH" {
		t.Errorf("Code = %q, want TEST_AUTH", s.Code)
	}
}

func TestClassifyError_UnregisteredFallsBackToInternal(t *testing.T) {
	t.Parallel()
	s := ClassifyError(errTestUnknown)
	if s == nil {
		t.Fatal("ClassifyError returned nil")
	}
	if s.Category != CategoryInternal {
		t.Errorf("Category = %q, want %q", s.Category, CategoryInternal)
	}
	if s.Code != "" {
		t.Errorf("Code = %q, want empty for unclassified", s.Code)
	}
}

func TestClassifyError_ExistingStructuredPreserved(t *testing.T) {
	t.Parallel()
	orig := &Structured{
		Category: CategoryParse,
		FilePath: "foo.yaml",
		Line:     42,
		Message:  "bad syntax",
		Hint:     "original hint",
		Code:     "ORIG_CODE",
	}
	got := ClassifyError(orig)
	if got == nil {
		t.Fatal("ClassifyError returned nil")
	}
	if got.FilePath != "foo.yaml" || got.Line != 42 {
		t.Errorf("location lost: FilePath=%q Line=%d", got.FilePath, got.Line)
	}
	if got.Code != "ORIG_CODE" || got.Hint != "original hint" {
		t.Errorf("fields overwritten: Code=%q Hint=%q", got.Code, got.Hint)
	}
}

func TestClassifyError_ExistingStructuredEnrichedFromRegistry(t *testing.T) {
	t.Parallel()
	orig := &Structured{
		Category: CategoryParse,
		FilePath: "foo.yaml",
		Line:     42,
		Message:  "bad syntax",
		Inner:    errTestOverride,
		// Code and Hint deliberately empty — should be enriched from registry
	}
	got := ClassifyError(orig)
	if got == nil {
		t.Fatal("ClassifyError returned nil")
	}
	if got.FilePath != "foo.yaml" || got.Line != 42 {
		t.Errorf("location lost: FilePath=%q Line=%d", got.FilePath, got.Line)
	}
	if got.Code != "TEST_OVERRIDE" {
		t.Errorf("Code not enriched: got %q, want TEST_OVERRIDE", got.Code)
	}
	if got.Hint != "Registered hint" {
		t.Errorf("Hint not enriched: got %q", got.Hint)
	}
}

func TestClassifyError_NetworkErrorClassified(t *testing.T) {
	t.Parallel()
	neterr := &NetworkError{
		Kind:    NetworkDNS,
		Message: "DNS resolution failed",
		Hint:    "Check hostname",
	}
	s := ClassifyError(neterr)
	if s == nil {
		t.Fatal("ClassifyError returned nil for NetworkError")
	}
	if s.Category != CategoryNetwork {
		t.Errorf("Category = %q, want network", s.Category)
	}
	if s.Code != "NETWORK_DNS" {
		t.Errorf("Code = %q, want NETWORK_DNS", s.Code)
	}
	if s.Message != "DNS resolution failed" {
		t.Errorf("Message = %q", s.Message)
	}
	if s.Hint != "Check hostname" {
		t.Errorf("Hint = %q", s.Hint)
	}
}

func TestClassifyError_WrappedNetworkError(t *testing.T) {
	t.Parallel()
	neterr := &NetworkError{Kind: NetworkTimeout, Message: "timed out", Hint: "raise timeout"}
	wrapped := fmt.Errorf("while calling remote: %w", neterr)
	s := ClassifyError(wrapped)
	if s == nil {
		t.Fatal("ClassifyError returned nil")
	}
	if s.Category != CategoryNetwork {
		t.Errorf("Category = %q, want network", s.Category)
	}
	if s.Code != "NETWORK_TIMEOUT" {
		t.Errorf("Code = %q, want NETWORK_TIMEOUT", s.Code)
	}
}

func TestClassifyError_ClassifyNetworkErrorCompatibility(t *testing.T) {
	t.Parallel()
	// A real DNS error — ClassifyNetworkError produces a NetworkError which
	// ClassifyError should handle identically.
	dnsErr := &net.DNSError{Name: "nonexistent.invalid", Err: "no such host"}
	ne := ClassifyNetworkError(dnsErr)
	s := ClassifyError(ne)
	if s.Category != CategoryNetwork || s.Code != "NETWORK_DNS" {
		t.Errorf("DNS not classified as NETWORK_DNS: %+v", s)
	}
}

func TestClassifyError_DeadlineExceededRoundTrip(t *testing.T) {
	t.Parallel()
	ne := ClassifyNetworkError(context.DeadlineExceeded)
	s := ClassifyError(ne)
	if s.Category != CategoryNetwork || s.Code != "NETWORK_TIMEOUT" {
		t.Errorf("deadline not classified as NETWORK_TIMEOUT: %+v", s)
	}
}

func TestLookupHint_NotRegistered(t *testing.T) {
	t.Parallel()
	if _, ok := LookupHint(errTestUnknown); ok {
		t.Fatal("LookupHint returned true for unregistered error")
	}
}

func TestLookupHint_Registered(t *testing.T) {
	t.Parallel()
	hint, ok := LookupHint(errTestParse)
	if !ok {
		t.Fatal("LookupHint returned false for registered error")
	}
	if hint.Code != "TEST_PARSE" {
		t.Errorf("Code = %q, want TEST_PARSE", hint.Code)
	}
}

func TestLookupHint_Wrapped(t *testing.T) {
	t.Parallel()
	wrapped := fmt.Errorf("outer: %w", errTestParse)
	hint, ok := LookupHint(wrapped)
	if !ok {
		t.Fatal("LookupHint returned false for wrapped sentinel")
	}
	if hint.Code != "TEST_PARSE" {
		t.Errorf("Code = %q", hint.Code)
	}
}

func TestIsRegistered(t *testing.T) {
	t.Parallel()
	if !IsRegistered(errTestParse) {
		t.Error("IsRegistered returned false for known sentinel")
	}
	if IsRegistered(errTestUnknown) {
		t.Error("IsRegistered returned true for unknown error")
	}
}

func TestRegisterPackage_Idempotent(t *testing.T) {
	t.Parallel()
	dup := stderrors.New("test: duplicate")
	RegisterPackage("classify_test_dup",
		RegisteredError{Name: "dup", Err: dup, Hint: ClassifiedHint{Category: CategoryInput, Code: "DUP1", Hint: "one"}},
	)
	RegisterPackage("classify_test_dup",
		RegisteredError{Name: "dup", Err: dup, Hint: ClassifiedHint{Category: CategoryInput, Code: "DUP2", Hint: "two"}},
	)
	hint, ok := LookupHint(dup)
	if !ok {
		t.Fatal("LookupHint false after double registration")
	}
	if hint.Code != "DUP2" {
		t.Errorf("Code = %q, want DUP2 (last registration wins)", hint.Code)
	}
}

func TestRegisteredPackages_ContainsRegistrations(t *testing.T) {
	t.Parallel()
	pkgs := RegisteredPackages()
	found := false
	for _, p := range pkgs {
		if p == "classify_test" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RegisteredPackages() missing classify_test: %v", pkgs)
	}
}

func TestPackageRegistrations_ReturnsEntries(t *testing.T) {
	t.Parallel()
	entries := PackageRegistrations("classify_test")
	if len(entries) < 4 {
		t.Fatalf("PackageRegistrations returned %d entries, want >= 4", len(entries))
	}
	// Verify one known entry.
	for _, e := range entries {
		if stderrors.Is(e.Err, errTestParse) {
			if e.Hint.Code != "TEST_PARSE" {
				t.Errorf("errTestParse registered with Code=%q", e.Hint.Code)
			}
			return
		}
	}
	t.Error("errTestParse not found in PackageRegistrations")
}

// typeClassifierTestErr is a package-level error type used only by
// TestRegisterTypeClassifier. It simulates a typed error (like *auth.GateError)
// that cannot be registered as a sentinel (no comparable value).
type typeClassifierTestErr struct{ msg string }

func (e *typeClassifierTestErr) Error() string { return e.msg }

// typeClassifierWrappedErr is a package-level error type used only by
// TestRegisterTypeClassifier_WrappedError. Implements error via Unwrap.
type typeClassifierWrappedErr struct{ inner error }

func (e *typeClassifierWrappedErr) Error() string { return "wrapped: " + e.inner.Error() }
func (e *typeClassifierWrappedErr) Unwrap() error { return e.inner }

func init() {
	// Register type classifiers for the test error types at init time so
	// they are available to all sub-tests without per-test registration races.
	RegisterTypeClassifier(
		func(err error) bool {
			var pe *typeClassifierTestErr
			return stderrors.As(err, &pe)
		},
		ClassifiedHint{
			Category: CategoryInput,
			Code:     "TEST_TYPE_CLASSIFIER",
			Hint:     "Set the missing value.",
		},
	)
	RegisterTypeClassifier(
		func(err error) bool {
			var wp *typeClassifierWrappedErr
			return stderrors.As(err, &wp)
		},
		ClassifiedHint{
			Category: CategoryAuth,
			Code:     "TEST_WRAPPED_TYPE",
			Hint:     "Upgrade to a plan that includes this feature.",
		},
	)
}

// TestRegisterTypeClassifier verifies that type-based classifiers added via
// RegisterTypeClassifier are consulted by LookupHint and ClassifyError when the
// sentinel registry does not match.
func TestRegisterTypeClassifier(t *testing.T) {
	e1 := &typeClassifierTestErr{msg: "boom"}

	t.Run("LookupHint finds typed error", func(t *testing.T) {
		hint, ok := LookupHint(e1)
		if !ok {
			t.Fatal("LookupHint returned false for registered typed error")
		}
		if hint.Code != "TEST_TYPE_CLASSIFIER" {
			t.Errorf("Code = %q, want TEST_TYPE_CLASSIFIER", hint.Code)
		}
	})

	t.Run("ClassifyError uses type classifier", func(t *testing.T) {
		s := ClassifyError(e1)
		if s == nil {
			t.Fatal("ClassifyError returned nil")
		}
		if s.Code != "TEST_TYPE_CLASSIFIER" {
			t.Errorf("Code = %q, want TEST_TYPE_CLASSIFIER", s.Code)
		}
		if s.Category != CategoryInput {
			t.Errorf("Category = %q, want input", s.Category)
		}
	})

	t.Run("sentinel registry takes priority over type classifier", func(t *testing.T) {
		// errTestParse is in the sentinel registry; the type classifier for
		// *typeClassifierTestErr does not match errTestParse, so the sentinel hint wins.
		s := ClassifyError(errTestParse)
		if s.Code != "TEST_PARSE" {
			t.Errorf("Code = %q, want TEST_PARSE (sentinel should win)", s.Code)
		}
	})
}

// TestRegisterTypeClassifier_WrappedError verifies that the type classifier
// matcher receives the full error chain via errors.As.
func TestRegisterTypeClassifier_WrappedError(t *testing.T) {
	e := &typeClassifierWrappedErr{inner: stderrors.New("inner")}
	wrapped := fmt.Errorf("outer: %w", e)
	hint, ok := LookupHint(wrapped)
	if !ok {
		t.Fatal("LookupHint returned false for wrapped typed error")
	}
	if hint.Code != "TEST_WRAPPED_TYPE" {
		t.Errorf("Code = %q, want TEST_WRAPPED_TYPE", hint.Code)
	}
}
