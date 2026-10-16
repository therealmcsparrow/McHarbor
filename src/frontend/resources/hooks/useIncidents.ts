// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ApiResponse, type PaginatedData } from '@core/api/client';
import { assertSuccess, describeApiError } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

export type Signal = 'crash_loop' | 'exit_nonzero' | 'oom' | 'unhealthy' | 'alert';
export type NarrativeStatus = 'none' | 'pending' | 'ready' | 'failed' | 'refused';
export type Confidence = 'low' | 'medium' | 'high';

export type MetricPoint = { at: string; value: number };

export type EvidenceSection = {
  title: string;
  kind: 'log' | 'table' | 'text' | (string & {});
  lines?: string[];
  columns?: string[];
  rows?: string[][];
  note?: string;
};

export type Evidence = {
  windowStart: string;
  windowEnd: string;
  container: {
    name: string;
    image?: string;
    imageId?: string;
    state?: string;
    exitCode?: number;
    oomKilled?: boolean;
    restarts?: number;
    health?: string;
    stack?: string;
  };
  events: { at: string; action: string; exitCode?: string; image?: string }[];
  logs: string[];
  logsNote?: string;
  subject?: { type: string; id: string; name: string; source: string; facts?: Record<string, string> };
  sections?: EvidenceSection[];
  imageChanges: { at: string; from: string; to: string }[];
  audit: { at: string; who: string; action: string; entity: string }[];
  deploys: { at: string; pipeline: string; stage: string; commit: string; status: string }[];
  alerts: { at: string; alert: string; state: string; message: string }[];
  hostCpu: MetricPoint[];
  hostMem: MetricPoint[];
  containerCpu: MetricPoint[];
  containerMem: MetricPoint[];
  drift: { watch: string; field: string; expected: string; actual: string }[];
  truncated?: boolean;
};

export type Narrative = {
  summary: string;
  startedAt?: string;
  changes: string[];
  causes: { title: string; evidence: string[]; confidence: Confidence }[];
  confidence: Confidence;
  undetermined: boolean;
};

export type ModelMeta = {
  provider: string;
  connection: string;
  model: string;
  requestedBy: string;
  at: string;
  promptSha256: string;
  proactive: boolean;
};

export type TimelineItem = { at: string; kind: string; who?: string; detail: string };

export type Incident = {
  id: string;
  envId: string;
  subjectType: string;
  subjectName: string;
  subjectId?: string;
  source?: string;
  stackName?: string;
  signal: Signal | (string & {});
  status: 'open' | 'resolved';
  signalCount: number;
  firstSignalAt: string;
  lastSignalAt: string;
  alertEventId?: string;
  evidence?: Evidence;
  evidenceAt?: string;
  narrative?: Narrative;
  narrativePrompt?: string;
  narrativeMeta?: ModelMeta;
  narrativeStatus: NarrativeStatus;
  narrativeError?: string;
  narratedAt?: string;
  timeline?: TimelineItem[];
  summary?: string;
  resolvedAt?: string;
  createdAt: string;
};

export class IncidentRefusal extends Error {
  constructor(
    readonly reason: string,
    message: string,
  ) {
    super(message);
  }
}

export function refusalOf<T>(res: ApiResponse<T>): T {
  if (!res.success) {
    const data = res.data as { reason?: unknown } | undefined;
    if (typeof data?.reason === 'string') throw new IncidentRefusal(data.reason, describeApiError(res));
  }
  return assertSuccess(res);
}

export function reasonOf(error: unknown): string | undefined {
  return error instanceof IncidentRefusal ? error.reason : undefined;
}

const KEY = ['incidents'] as const;

export function useIncidents(currentStatus: 'open' | 'resolved' | '') {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: [...KEY, 'list', currentStatus, envId],
    queryFn: () =>
      api
        .get<PaginatedData<Incident>>('/incidents', {
          per_page: '200',
          ...(currentStatus ? { status: currentStatus } : {}),
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 20_000,
  });
}

export function useIncident(id: string | undefined) {
  return useQuery({
    queryKey: [...KEY, 'one', id],
    enabled: Boolean(id),
    queryFn: () => api.get<Incident>(`/incidents/${encodeURIComponent(id ?? '')}`).then(assertSuccess),
    refetchInterval: (query) => (query.state.data?.narrativeStatus === 'pending' ? 3_000 : 20_000),
  });
}

export function useIncidentForAlert(eventId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: [...KEY, 'alert', eventId],
    enabled: enabled && Boolean(eventId),
    queryFn: () =>
      api
        .get<Incident>(`/incidents/by-alert/${encodeURIComponent(eventId ?? '')}`)
        .then((r) => (r.success ? (r.data ?? null) : null)),
    staleTime: 30_000,
  });
}

function useIncidentAction(path: 'narrate' | 'summarise' | 'resolve' | 'evidence') {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<Incident>(`/incidents/${encodeURIComponent(id)}/${path}`).then(refusalOf),
    onSuccess: (incident) => {
      client.setQueryData([...KEY, 'one', incident.id], incident);
      void client.invalidateQueries({ queryKey: [...KEY, 'list'] });
    },
    onError: (_err) => void client.invalidateQueries({ queryKey: KEY }),
  });
}

export const useNarrate = () => useIncidentAction('narrate');
export const useSummarise = () => useIncidentAction('summarise');
export const useResolveIncident = () => useIncidentAction('resolve');
export const useRefreshEvidence = () => useIncidentAction('evidence');
