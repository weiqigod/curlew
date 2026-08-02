package auth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/weiqigod/curlew/internal/variable"
)

// Sentinel errors for auth profile failures.
var (
	ErrProfileFailed    = errors.New("auth profile execution failed")
	ErrProfileNoExtract = errors.New("auth profile produced no variables")
)

// ProfileType identifies the kind of auth profile.
type ProfileType string

// ProfileDynamic executes a collection to obtain credentials.
const ProfileDynamic ProfileType = "dynamic"

// Profile represents a single auth profile entry from curlew.yaml.
type Profile struct {
	Name             string      // profile key name (e.g., "admin_token")
	Type             ProfileType // "dynamic"
	Collection       string      // relative path to collection file
	Extract          string      // variable name to extract (empty = all extracted vars)
	CacheTTL         int         // cache_ttl in seconds; 0 = no caching (default)
	RefreshOnFailure bool        // retry with fresh auth on 401 responses
}

// ProfileResult holds the outcome of executing all auth profiles.
type ProfileResult struct {
	Variables map[string]string
	Sensitive *variable.SensitiveSet
}

// ExecuteFunc executes a collection at the given path and returns extracted variables.
type ExecuteFunc func(ctx context.Context, collectionPath string) (map[string]string, error)

// ExecuteProfiles runs all auth profiles sequentially and returns merged variables.
// All returned variables are automatically marked as sensitive.
// If cache is nil, a NopCacheStore is used (no caching).
// Returns ErrProfileFailed if any profile execution fails.
func ExecuteProfiles(ctx context.Context, profiles []Profile, projectRoot string, execute ExecuteFunc, cache CacheStore) (*ProfileResult, error) {
	if cache == nil {
		cache = NopCacheStore{}
	}
	result := &ProfileResult{
		Variables: make(map[string]string),
		Sensitive: variable.NewSensitiveSet(),
	}
	if len(profiles) == 0 {
		return result, nil
	}

	for _, p := range profiles {
		// Cache check: only when TTL is configured.
		if p.CacheTTL > 0 {
			if entry, err := cache.Load(p.Name); err == nil && entry.Collection == p.Collection {
				// Cache hit: deobfuscate and apply to result.
				hit := true
				for k, v := range entry.Variables {
					plain, deErr := Deobfuscate(v, p.Name)
					if deErr != nil {
						_ = cache.Invalidate(p.Name)
						hit = false
						break
					}
					result.Variables[k] = plain
					result.Sensitive.Add(k)
				}
				if hit {
					continue
				}
			}
		}

		collPath := p.Collection
		if projectRoot != "" && !filepath.IsAbs(collPath) {
			collPath = filepath.Join(projectRoot, collPath)
		}

		vars, err := execute(ctx, collPath)
		if err != nil {
			return nil, fmt.Errorf("auth profile %q: %w: %w", p.Name, ErrProfileFailed, err)
		}

		var extracted map[string]string
		if p.Extract != "" {
			val, ok := vars[p.Extract]
			if !ok {
				return nil, fmt.Errorf("auth profile %q: %w: variable %q not found in collection output",
					p.Name, ErrProfileNoExtract, p.Extract)
			}
			extracted = map[string]string{p.Extract: val}
		} else {
			extracted = vars
		}

		for k, v := range extracted {
			result.Variables[k] = v
			result.Sensitive.Add(k)
		}

		// Cache save (if TTL > 0) — best-effort; never fail the run.
		if p.CacheTTL > 0 {
			obfVars := make(map[string]string, len(extracted))
			for k, v := range extracted {
				obfVars[k] = Obfuscate(v, p.Name)
			}
			entry := &CacheEntry{
				ProfileName: p.Name,
				Variables:   obfVars,
				ExpiresAt:   time.Now().Add(time.Duration(p.CacheTTL) * time.Second),
				CreatedAt:   time.Now(),
				Collection:  p.Collection,
			}
			_ = cache.Save(entry)
		}
	}

	return result, nil
}
