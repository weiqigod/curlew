package uiserver

import (
	"net/http"
	"time"

	"github.com/peterlindqvist/apitest/internal/output/events"
)

type metaServer struct {
	Version             string `json:"version"`
	APIVersion          int    `json:"api_version"`
	EventsSchemaVersion string `json:"events_schema_version"`
	StartedAt           string `json:"started_at"`
}

type metaProject struct {
	Root             string  `json:"root"`
	Name             string  `json:"name"`
	DefaultEnv       string  `json:"default_env"`
	CollectionFilter *string `json:"collection_filter"`
}

type metaHistory struct {
	Enabled bool `json:"enabled"`
	MaxRuns int  `json:"max_runs"`
}

type metaLimits struct {
	InlineBodyBytes int `json:"inline_body_bytes"`
	StoredBodyBytes int `json:"stored_body_bytes"`
	EventBodyBytes  int `json:"event_body_bytes"`
	MemoryRuns      int `json:"memory_runs"`
}

type metaResponse struct {
	Server  metaServer  `json:"server"`
	Project metaProject `json:"project"`
	History metaHistory `json:"history"`
	Limits  metaLimits  `json:"limits"`
}

// handleMeta serves GET /api/v1/meta (spec §4.2). Never included:
// environment variable values, os.Environ, absolute paths outside the
// project root.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	var filter *string
	if s.opts.CollectionFilter != "" {
		f := s.opts.CollectionFilter
		filter = &f
	}
	writeJSON(w, http.StatusOK, metaResponse{
		Server: metaServer{
			Version:             s.opts.Version,
			APIVersion:          APIVersion,
			EventsSchemaVersion: events.SchemaVersion,
			StartedAt:           s.startedAt.Format(time.RFC3339),
		},
		Project: metaProject{
			Root:             s.opts.Root,
			Name:             s.opts.ProjectName,
			DefaultEnv:       s.opts.DefaultEnv,
			CollectionFilter: filter,
		},
		History: metaHistory{
			Enabled: s.opts.HistoryEnabled,
			MaxRuns: s.opts.MaxRuns,
		},
		Limits: metaLimits{
			InlineBodyBytes: InlineBodyLimit,
			StoredBodyBytes: StoredBodyLimit,
			EventBodyBytes:  events.DefaultBodyLimit,
			MemoryRuns:      MemoryRuns,
		},
	})
}
