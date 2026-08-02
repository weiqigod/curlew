package backend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

// ErrTeamVaultNotFound is returned when the backend has no vault-config row for
// the given orgId (HTTP 404). Callers treat this as "team vault not provisioned
// yet" — not an error to surface to the user.
var ErrTeamVaultNotFound = errors.New("backend: team vault not configured for organization")

// ErrOrgIDRequired is returned when GetTeamVault is called with an empty orgId.
var ErrOrgIDRequired = errors.New("backend: orgId is required")

// orgIDRE validates that orgId contains only safe identifier characters,
// preventing path-traversal bugs if a malformed JWT claim is ever parsed.
var orgIDRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// TeamVaultResponse is the decoded body of GET /api/v1/organizations/{orgId}/vault-config.
// The Template field is the raw YAML string the backend stored; the CLI parses
// it via teamtemplate.Parse downstream.
type TeamVaultResponse struct {
	Template       string `json:"template"`
	Version        int64  `json:"version"`
	UpdatedAt      string `json:"updated_at,omitempty"`
	UpdatedByEmail string `json:"updated_by_email,omitempty"`
}

// GetTeamVault fetches the shared vault configuration for orgId.
// Returns ErrTeamVaultNotFound on 404; passes other transport/problem-details
// errors through unchanged so callers can errors.Is/As them with the existing
// sentinels (ErrNetworkFailure, ErrServerError, *ProblemDetails).
func (c *Client) GetTeamVault(ctx context.Context, orgId, accessToken string) (*TeamVaultResponse, error) {
	if orgId == "" {
		return nil, ErrOrgIDRequired
	}
	if !orgIDRE.MatchString(orgId) {
		return nil, fmt.Errorf("backend: invalid orgId %q: must match ^[a-zA-Z0-9_-]+$", orgId)
	}
	path := fmt.Sprintf("/api/v1/organizations/%s/vault-config", orgId)
	var out TeamVaultResponse
	err := c.GetJSON(ctx, path, accessToken, &out)
	if err != nil {
		var pd *ProblemDetails
		if errors.As(err, &pd) && pd.Status == http.StatusNotFound {
			return nil, ErrTeamVaultNotFound
		}
		return nil, err
	}
	return &out, nil
}
