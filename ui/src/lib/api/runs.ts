import { apiFetch, apiFetchRaw } from './client';
import type {
  CurrentRunResponse,
  RequestDetail,
  RequestListResponse,
  RunInfo,
  RunListResponse,
  StartParams,
  StartRunResponse,
} from '../types/run';

/** POST /api/v1/runs (§4.7) — 202 on success. */
export function startRun(params: StartParams): Promise<StartRunResponse> {
  return apiFetch<StartRunResponse>('/runs', {
    method: 'POST',
    body: JSON.stringify(params),
  });
}

/** GET /api/v1/runs/current (§4.8). */
export function getCurrentRun(): Promise<CurrentRunResponse> {
  return apiFetch<CurrentRunResponse>('/runs/current');
}

/** GET /api/v1/runs/{run_id} (§4.8). */
export function getRun(runId: string): Promise<RunInfo> {
  return apiFetch<RunInfo>(`/runs/${encodeURIComponent(runId)}`);
}

/** GET /api/v1/runs/{run_id}/requests — the light list (§4.8). */
export function getRunRequests(runId: string): Promise<RequestListResponse> {
  return apiFetch<RequestListResponse>(`/runs/${encodeURIComponent(runId)}/requests`);
}

/** GET /api/v1/runs/{run_id}/requests/{request_id} — full detail (§4.9). */
export function getRequestDetail(runId: string, requestId: string): Promise<RequestDetail> {
  return apiFetch<RequestDetail>(
    `/runs/${encodeURIComponent(runId)}/requests/${encodeURIComponent(requestId)}`,
  );
}

/** GET .../body?which=request|response (§4.10) — full redacted body bytes. */
export function getRequestBody(
  runId: string,
  requestId: string,
  which: 'request' | 'response',
): Promise<Response> {
  return apiFetchRaw(
    `/runs/${encodeURIComponent(runId)}/requests/${encodeURIComponent(requestId)}/body?which=${which}`,
  );
}

/** POST /api/v1/runs/{run_id}/cancel (§4.8) — 202, idempotent while cancelling. */
export function cancelRun(runId: string): Promise<{ state: string }> {
  return apiFetch<{ state: string }>(`/runs/${encodeURIComponent(runId)}/cancel`, {
    method: 'POST',
  });
}

/** GET /api/v1/runs?limit=&offset= (§4.11). */
export function listRuns(limit = 50, offset = 0): Promise<RunListResponse> {
  return apiFetch<RunListResponse>(`/runs?limit=${limit}&offset=${offset}`);
}

/** DELETE /api/v1/runs/{run_id} (§4.11, Solo). */
export function deleteRun(runId: string): Promise<void> {
  return apiFetch<void>(`/runs/${encodeURIComponent(runId)}`, { method: 'DELETE' });
}
