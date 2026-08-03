package teamtemplate

import (
	"fmt"
	"io"
	"os"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// LoadOptions parameterises Load. The zero value behaves as "no local
// template, no template returned".
type LoadOptions struct {
	// LocalPath is the CURLEW_TEAM_CONFIG path; empty = no template.
	LocalPath string
	// Warn receives diagnostics; nil = io.Discard.
	Warn io.Writer
}

// LoadResult bundles the loaded template with diagnostics for the run header.
type LoadResult struct {
	// Template is the loaded template; nil when LocalPath produced none.
	Template *TeamTemplate
	// LocalOverlay is true when LocalPath contributed keys.
	LocalOverlay bool
}

// Load reads the shared vault template from LocalPath. Curlew has no vault
// backend: the local file is the only source, so there is nothing to merge.
//
// Returns a nil LoadResult when LocalPath is empty (no error).
func Load(opts LoadOptions) (*LoadResult, error) {
	if opts.LocalPath == "" {
		return nil, nil
	}

	data, readErr := os.ReadFile(opts.LocalPath)
	if readErr != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  fmt.Sprintf("shared vault template not found: %s", opts.LocalPath),
			Hint:     "Check CURLEW_TEAM_CONFIG or remove it to disable team templates",
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

	return &LoadResult{Template: tpl, LocalOverlay: true}, nil
}
