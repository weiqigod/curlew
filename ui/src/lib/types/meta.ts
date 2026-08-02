// 1:1 mirrors of GET /api/v1/meta (UI_SPECIFICATION.md §4.2).
// Field names stay snake_case matching the wire format — no mapping layer.

export interface MetaServer {
  version: string;
  api_version: number;
  events_schema_version: string;
  started_at: string;
}

export interface MetaProject {
  root: string;
  name: string;
  default_env: string;
  collection_filter: string | null;
}

export interface MetaHistory {
  enabled: boolean;
  max_runs: number;
}

export interface MetaLimits {
  inline_body_bytes: number;
  stored_body_bytes: number;
  event_body_bytes: number;
  memory_runs: number;
}

export interface Meta {
  server: MetaServer;
  project: MetaProject;
  history: MetaHistory;
  limits: MetaLimits;
}
