package config

import (
	"errors"
	"fmt"
	"os"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/vault/teamtemplate"
)

// LoadTeamTemplate reads, parses, and validates the shared vault template at
// the given path. Returns a structured error ready for rendering by the CLI
// printers. An empty path returns (nil, nil) — the caller decides whether that
// is an error.
func LoadTeamTemplate(path string) (*teamtemplate.TeamTemplate, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &apierrors.Structured{
				Category: apierrors.CategoryConfig,
				Message:  fmt.Sprintf("shared vault template not found: %s", path),
				Hint:     "Check CURLEW_TEAM_CONFIG or remove it to disable team templates",
				Inner:    teamtemplate.ErrTemplateNotFound,
			}
		}
		return nil, fmt.Errorf("read team template %q: %w", path, err)
	}

	tpl, parseErr := teamtemplate.Parse(data)
	if parseErr != nil {
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  fmt.Sprintf("parse team template %s: %v", path, parseErr),
			Inner:    teamtemplate.ErrInvalidTemplate,
		}
	}

	if issues := tpl.Validate(); len(issues) > 0 {
		first := issues[0]
		return nil, &apierrors.Structured{
			Category: apierrors.CategoryConfig,
			Message:  fmt.Sprintf("invalid team template %s: %s: %s", path, first.Path, first.Message),
			Inner:    teamtemplate.ErrInvalidTemplate,
		}
	}

	return tpl, nil
}
