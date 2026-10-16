// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { ApiError, summarizeApiError } from '@resources/utils/api-mutation';

import type {
  AIConnectionsResponse,
  CatalogResponse,
  DeliveryTargetsResponse,
  GenerateJob,
  GenerateRequest,
  ListResponse,
  Schedule,
  ScheduleListResponse,
  ScheduleRequest,
  ScheduleRun,
  Settings,
  StoredReport,
  SubjectKind,
  SubjectsResponse,
} from './types';

// The plugin's API client.
//
// Every call goes through the host's session cookie (`credentials:
// 'include'`), which is what makes a report contain only what the
// signed-in user may already see. There is no token here to leak because
// there is no token.

const BASE = '/api/reporting';

type Envelope<T> = {
  success: boolean;
  data?: T;
  error?: string;
  code?: string;
  detail?: string;
  requestId?: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers({ 'Content-Type': 'application/json', Accept: 'application/json' });
  new Headers(init?.headers).forEach((value, key) => headers.set(key, value));
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: 'include',
    headers,
  });

  let body: Envelope<T> | null;
  try {
    body = (await res.json()) as Envelope<T>;
  } catch {
    body = null;
  }

  if (!res.ok || !body?.success) {
    const code = body?.code ?? 'err.reporting.internal';
    const summary = summarizeApiError({ code, error: body?.error });
    throw new ApiError(summary, res.status, {
      summary,
      detail: body?.detail?.trim() || undefined,
      code,
      requestId: body?.requestId,
      status: res.status,
    });
  }
  return body.data as T;
}

export function fetchCatalog(): Promise<CatalogResponse> {
  return request<CatalogResponse>('/catalog');
}

export function fetchReports(page: number, perPage: number, type?: string): Promise<ListResponse> {
  const params = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  if (type) params.set('type', type);
  return request<ListResponse>(`/reports?${params.toString()}`);
}

export function fetchReport(id: string): Promise<StoredReport> {
  return request<StoredReport>(`/reports/${encodeURIComponent(id)}`);
}

/** The objects a report may be narrowed to, as this user may see them. */
export function fetchSubjects(kind: SubjectKind, envId: string): Promise<SubjectsResponse> {
  const params = new URLSearchParams({ kind });
  if (envId) params.set('env', envId);
  return request<SubjectsResponse>(`/subjects?${params.toString()}`);
}

/**
 * Generation, as a job.
 *
 * The report itself takes tens of seconds — twenty loopback calls, several
 * of them over the agent transport — so the request that starts it returns
 * at once and the studio polls. That is what lets the configure dialog
 * close on Generate instead of freezing with a spinner in its footer.
 */
export function startGenerateJob(body: GenerateRequest): Promise<GenerateJob> {
  return request<GenerateJob>('/jobs', { method: 'POST', body: JSON.stringify(body) });
}

export function fetchGenerateJob(id: string): Promise<GenerateJob> {
  return request<GenerateJob>(`/jobs/${encodeURIComponent(id)}`);
}

export function deleteReport(id: string): Promise<void> {
  return request<void>(`/reports/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/**
 * The AI connections the studio may offer in a "which AI should write
 * this report" selector. Read-only — it never creates, edits or deletes
 * a connection, that stays in AI settings.
 */
export function fetchAIConnections(): Promise<AIConnectionsResponse> {
  return request<AIConnectionsResponse>('/ai/connections');
}

// --- schedules ---------------------------------------------------------

export function fetchSchedules(page: number, perPage: number): Promise<ScheduleListResponse> {
  const params = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  return request<ScheduleListResponse>(`/schedules?${params.toString()}`);
}

export function createSchedule(body: ScheduleRequest): Promise<Schedule> {
  return request<Schedule>('/schedules', { method: 'POST', body: JSON.stringify(body) });
}

export function updateSchedule(id: string, body: ScheduleRequest): Promise<Schedule> {
  return request<Schedule>(`/schedules/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  });
}

export function deleteSchedule(id: string): Promise<void> {
  return request<void>(`/schedules/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/**
 * Fire a schedule now, without disturbing its cron.
 *
 * The backend answers 202 as soon as the run is claimed — a generation
 * takes tens of seconds, and holding the request open for it would be the
 * same mistake the studio's job endpoint exists to avoid. The page polls
 * the schedule list for the outcome.
 */
export function runScheduleNow(id: string): Promise<void> {
  return request<void>(`/schedules/${encodeURIComponent(id)}/run`, { method: 'POST' });
}

/** The stored last run, with its document. */
export function fetchScheduleRun(id: string): Promise<ScheduleRun> {
  return request<ScheduleRun>(`/schedules/${encodeURIComponent(id)}/run`);
}

/** The channels and storage locations a schedule may deliver to. */
export function fetchDeliveryTargets(): Promise<DeliveryTargetsResponse> {
  return request<DeliveryTargetsResponse>('/delivery-targets');
}

export function fetchSettings(): Promise<Settings> {
  return request<Settings>('/settings');
}

export function saveSettings(body: Settings): Promise<Settings> {
  return request<Settings>('/settings', { method: 'PUT', body: JSON.stringify(body) });
}

/**
 * Download an export.
 *
 * The response is fetched rather than linked so a failure surfaces as an
 * error the user can see, instead of the browser quietly navigating to a
 * JSON error page and losing the page they were on. The object URL is
 * revoked on the next tick — long enough for the download to start,
 * short enough not to pin a multi-megabyte blob in memory.
 */
export async function downloadExport(id: string, format: 'pdf' | 'docx', fallbackName: string): Promise<void> {
  const res = await fetch(`${BASE}/reports/${encodeURIComponent(id)}/export?format=${format}`, {
    credentials: 'include',
  });
  if (!res.ok) {
    throw new ApiError('err.reporting.export_failed', res.status);
  }

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filenameFrom(res.headers.get('Content-Disposition')) ?? `${fallbackName}.${format}`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 2000);
}

/**
 * Download a schedule's last run.
 *
 * Same shape as `downloadExport` and deliberately not folded into it: the
 * two hang off different resources, and a shared helper taking a path
 * would be one indirection for one saved line.
 */
export async function downloadScheduleRun(id: string, format: 'pdf' | 'docx', fallbackName: string): Promise<void> {
  const res = await fetch(`${BASE}/schedules/${encodeURIComponent(id)}/run/export?format=${format}`, {
    credentials: 'include',
  });
  if (!res.ok) {
    throw new ApiError('err.reporting.export_failed', res.status);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filenameFrom(res.headers.get('Content-Disposition')) ?? `${fallbackName}.${format}`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 2000);
}

function filenameFrom(header: string | null): string | null {
  if (!header) return null;
  const match = /filename="?([^";]+)"?/i.exec(header);
  return match?.[1] ?? null;
}
