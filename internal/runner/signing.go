package runner

import (
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/signer"
)

// resolveSigningSpec folds collection-level + per-request precedence to
// determine the effective signing spec for a single request:
//   - per-request explicit null (signing: ~) → nil (signing disabled)
//   - per-request non-nil, non-null → the request's own spec
//   - per-request nil + collection non-nil → the collection spec
//   - both nil → nil
func resolveSigningSpec(item parser.RequestItem, col *parser.Collection) *signer.RequestSpec {
	if item.Signing != nil {
		if item.Signing.IsExplicitNull() {
			return nil
		}
		return &signer.RequestSpec{Type: item.Signing.Type, Params: item.Signing.Params}
	}
	if col.Signing != nil && !col.Signing.IsExplicitNull() {
		return &signer.RequestSpec{Type: col.Signing.Type, Params: col.Signing.Params}
	}
	return nil
}

// collectionUsesSigning reports whether any request in any phase, or the
// collection itself, declares a non-null signing spec. Used for the
// exec-wrap fast path — when false, the signer wrap is never allocated.
func collectionUsesSigning(col *parser.Collection) bool {
	if col.Signing != nil && !col.Signing.IsExplicitNull() {
		return true
	}
	for _, sect := range []*parser.Section{&col.Setup, &col.Requests, &col.Teardown} {
		for i := range sect.Items {
			sp := sect.Items[i].Signing
			if sp != nil && !sp.IsExplicitNull() {
				return true
			}
		}
	}
	return false
}
