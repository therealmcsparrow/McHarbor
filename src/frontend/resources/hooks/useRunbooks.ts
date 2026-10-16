// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import i18n from 'i18next';
import { api, type ApiResponse, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useOnCallQuery } from '@resources/hooks/useOnCallQuery';
import type {
  ResolvedRunbooks,
  Runbook,
  RunbookCatalog,
  RunbookInput,
  RunbookRun,
  RunbookStepResult,
  RunbookTarget,
  StepState,
} from '@resources/types/runbooks';

export const RUNBOOKS_KEY = ['runbooks'] as const;

export class StepDeniedError extends Error {
  readonly permission: string;

  constructor(permission: string) {
    super(i18n.t('permission.requires', { ns: 'common', permission }));
    this.name = 'StepDeniedError';
    this.permission = permission;
  }
}

function resolveTargetParams(target: RunbookTarget): Record<string, string> {
  const params: Record<string, string> = {};
  if (target.environmentId) params.env = target.environmentId;
  if (target.sourceType) params.sourceType = target.sourceType;
  if (target.conditionKey) params.conditionKey = target.conditionKey;
  if (target.alertId) params.alertId = target.alertId;
  if (target.alertEventId) params.alertEventId = target.alertEventId;
  if (target.container) params.container = target.container;
  if (target.stack) params.stack = target.stack;
  if (target.kind && target.id) {
    params.kind = target.kind;
    params.id = target.id;
  }
  return params;
}

export function useResolvedRunbooks(target: RunbookTarget, enabled = true) {
  const params = resolveTargetParams(target);
  const query = useOnCallQuery<ResolvedRunbooks>(['runbooks', 'for', params], '/runbooks/for', params, {
    enabled: enabled && Object.keys(params).length > 0,
  });
  return {
    runbooks: query.data?.data.runbooks ?? [],
    openRuns: query.data?.data.openRuns ?? [],
    cachedAt: query.data?.cachedAt ?? null,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}

export function useRunbooks(envId?: string) {
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'list', envId ?? ''],
    queryFn: () =>
      api
        .get<PaginatedData<Runbook>>('/runbooks', { per_page: '200', ...(envId ? { env: envId } : {}) })
        .then((res) => assertSuccess(res).items ?? []),
  });
}

export function useRunbook(id: string | undefined) {
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'one', id ?? ''],
    enabled: Boolean(id),
    queryFn: () => api.get<Runbook>(`/runbooks/${encodeURIComponent(id ?? '')}`).then(assertSuccess),
  });
}

export function useRunbookCatalog() {
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'catalog'],
    staleTime: 5 * 60_000,
    queryFn: () => api.get<RunbookCatalog>('/runbooks/catalog').then(assertSuccess),
  });
}

export function useRunbookRun(id: string | undefined) {
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'run', id ?? ''],
    enabled: Boolean(id),
    queryFn: () => api.get<RunbookRun>(`/runbooks/runs/${encodeURIComponent(id ?? '')}`).then(assertSuccess),
  });
}

export function useSaveRunbook() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id?: string; input: RunbookInput }) =>
      (id
        ? api.put<Runbook>(`/runbooks/${encodeURIComponent(id)}`, input)
        : api.post<Runbook>('/runbooks', input)
      ).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: RUNBOOKS_KEY }),
  });
}

export function useDeleteRunbook() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/runbooks/${encodeURIComponent(id)}`).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: RUNBOOKS_KEY }),
  });
}

export function useStartRun() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      runbookId: string;
      environmentId?: string;
      subjectKind?: string;
      subjectId?: string;
      subjectName?: string;
      incidentId?: string;
      alertEventId?: string;
    }) => api.post<RunbookRun>('/runbooks/runs', input).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: RUNBOOKS_KEY }),
  });
}

// A refused step carries its own payload in `data`, which the envelope types as the success shape.
function stringField(data: unknown, field: string): string | undefined {
  if (typeof data !== 'object' || data === null) return undefined;
  const value: unknown = Reflect.get(data, field);
  return typeof value === 'string' ? value : undefined;
}

export function recordStep(res: ApiResponse<RunbookStepResult>): RunbookStepResult {
  if (!res.success && res.code === 'err.runbook.step_forbidden') {
    throw new StepDeniedError(stringField(res.data, 'permission') ?? '');
  }
  if (!res.success && res.code === 'err.runbook.step_failed') {
    throw new Error(stringField(res.data, 'lastError')?.trim() || (res.error ?? ''));
  }
  return assertSuccess(res);
}

export function useRecordStep(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (vars: { stepId: string; state: StepState; skipReason?: string; finding?: string }) =>
      api
        .post<RunbookStepResult>(
          `/runbooks/runs/${encodeURIComponent(runId)}/steps/${encodeURIComponent(vars.stepId)}`,
          {
            state: vars.state,
            skipReason: vars.skipReason ?? '',
            finding: vars.finding ?? '',
          },
        )
        .then(recordStep),
    onSettled: () => void client.invalidateQueries({ queryKey: [...RUNBOOKS_KEY, 'run', runId] }),
  });
}

export function useFinishRun(runId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (vars: { outcome: 'completed' | 'abandoned' | 'escalated'; note?: string }) =>
      api
        .post<RunbookRun>(`/runbooks/runs/${encodeURIComponent(runId)}/finish`, {
          outcome: vars.outcome,
          note: vars.note ?? '',
        })
        .then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: RUNBOOKS_KEY }),
  });
}
