package teamtemplate

import (
	"context"
	"fmt"
	"io"
	"os"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
)

// LoadOptions parameterises Load. Zero value behaves as "no backend, no local
// overlay, no template returned".
type LoadOptions struct {
	// Cache holds the backend-fetched cache. nil = no backend.
	Cache *Cache
	// Fetcher is used for stale-while-revalidate; nil = cache-only read.
	Fetcher Fetcher
	// OrgID is the organization identifier for backend fetch; empty = skip backend.
	OrgID string
	// AccessToken is the access token for backend fetch; empty = skip backend.
	AccessToken string
	// LocalPath is the APITEST_TEAM_CONFIG path; empty = no local overlay.
	LocalPath string
	// Force bypasses TTL (--refresh-vault).
	Force bool
	// Warn receives stale-cache warnings; nil = io.Discard.
	Warn io.Writer
}

// LoadResult bundles the merged template with diagnostics for the run header.
type LoadResult struct {
	// Template is the merged template; nil when neither source produced one.
	Template *TeamTemplate
	// BackendVersion is 0 when no backend source was used.
	BackendVersion int64
	// LocalOverlay is true when LocalPath contributed keys.
	LocalOverlay bool
}

// Load applies the precedence chain from SPECIFICATION.md:5699-5703:
//  1. backend-fetched template (base)
//  2. local file at LocalPath (overlay; per-key — local replaces backend at same env+key)
//
// Returns nil LoadResult when neither source produced a template (no error).
func Load(ctx context.Context, opts LoadOptions) (*LoadResult, error) {
	warn := opts.Warn
	if warn == nil {
		warn = io.Discard
	}

	var backendTpl *TeamTemplate
	var backendVersion int64

	// Step 1: try backend cache.
	if opts.Cache != nil {
		env, err := opts.Cache.Load(ctx, opts.Fetcher, opts.OrgID, opts.AccessToken, opts.Force, warn)
		if err == nil && env != nil {
			tpl, parseErr := Parse([]byte(env.Template))
			if parseErr == nil {
				backendTpl = tpl
				backendVersion = env.Version
			} else {
				_, _ = fmt.Fprintf(warn, "team vault cache parse error (ignoring): %v\n", parseErr)
			}
		}
		// Fetch errors are already handled (stale cache or warning) inside Cache.Load.
	}

	// Step 2: load local overlay.
	var localTpl *TeamTemplate
	var localOverlay bool
	if opts.LocalPath != "" {
		data, readErr := os.ReadFile(opts.LocalPath)
		if readErr != nil {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("shared vault template not found: %s", opts.LocalPath),
				Hint:     "Check APITEST_TEAM_CONFIG or remove it to disable team templates",
				Inner:    ErrTemplateNotFound,
			}
		}
		tpl, parseErr := Parse(data)
		if parseErr != nil {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("parse team template %s: %v", opts.LocalPath, parseErr),
				Inner:    ErrInvalidTemplate,
			}
		}
		localTpl = tpl
		localOverlay = true
	}

	// No template from either source.
	if backendTpl == nil && localTpl == nil {
		return nil, nil
	}

	// Merge: backend is base, local is overlay.
	var merged *TeamTemplate
	switch {
	case backendTpl != nil && localTpl != nil:
		merged = backendTpl.Merge(localTpl)
	case backendTpl != nil:
		merged = backendTpl
	default:
		merged = localTpl
	}

	return &LoadResult{
		Template:       merged,
		BackendVersion: backendVersion,
		LocalOverlay:   localOverlay,
	}, nil
}
