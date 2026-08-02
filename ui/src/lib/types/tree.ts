// 1:1 mirrors of GET /api/v1/tree, /environments, /validate
// (UI_SPECIFICATION.md §4.3–§4.5). snake_case kept.

export interface TreeIssue {
  severity: string;
  line: number;
  message: string;
  hint?: string;
}

export interface TreeRequest {
  name: string;
  slug: string;
  phase: string;
  method: string;
  /** Raw uninterpolated URL template — never resolved values. */
  url: string;
  source_line: number;
  data_driven: boolean;
  required: boolean;
}

export interface TreeCounts {
  setup: number;
  main: number;
  teardown: number;
}

export interface TreeCollection {
  path: string;
  name: string | null;
  valid: boolean;
  counts: TreeCounts | null;
  requests: TreeRequest[];
  issues: TreeIssue[];
}

export interface Tree {
  etag: string;
  collections: TreeCollection[];
}

// §4.4 GET /api/v1/environments
export interface EnvVariable {
  name: string;
  /** "[REDACTED]" when sensitive — the real value never reaches the client. */
  value: string;
  sensitive: boolean;
}

export interface Environment {
  name: string;
  file: string;
  variables: EnvVariable[];
}

export interface EnvironmentsResponse {
  environments: Environment[];
}

// §4.5 GET /api/v1/validate
export interface ValidateFile {
  file: string;
  valid: boolean;
  issues: TreeIssue[];
}

export interface ValidateResult {
  valid: boolean;
  files: ValidateFile[];
}
