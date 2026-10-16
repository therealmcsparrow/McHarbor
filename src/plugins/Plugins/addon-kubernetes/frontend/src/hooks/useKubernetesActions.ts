// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { ApiError, assertSuccess, summarizeApiError } from '@resources/utils/api-mutation';
import { clusterParams, clusterSuffix, useSelectedCluster } from '../cluster-store';

/** Identifies a namespaced object. */
export type ResourceRef = {
  namespace: string;
  name: string;
};

export type ScaleVars = ResourceRef & { replicas: number };

/**
 * Every mutation invalidates the summary, which is the single query backing all
 * thirteen tabs — there is no per-resource query to update surgically. The
 * summary also refetches on a 15s interval, so a missed invalidation would
 * still self-heal, just late enough for the reader to think the action failed.
 */
function useSummaryInvalidation() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: ['kubernetes', 'summary'] });
}

export function useScaleDeployment() {
  const { t } = useTranslation('kubernetes');
  const envId = useSelectedCluster();
  const invalidate = useSummaryInvalidation();
  return useMutation({
    mutationFn: ({ namespace, name: itemName, replicas }: ScaleVars) =>
      api
        .post(`/kubernetes/deployments/${namespace}/${itemName}/scale${clusterSuffix(envId)}`, { replicas })
        .then(assertSuccess),
    meta: { success: t('toast.scaled') },
    onSuccess: invalidate,
  });
}

export function useRestartDeployment() {
  const { t } = useTranslation('kubernetes');
  const envId = useSelectedCluster();
  const invalidate = useSummaryInvalidation();
  return useMutation({
    mutationFn: ({ namespace, name: itemName }: ResourceRef) =>
      api.post(`/kubernetes/deployments/${namespace}/${itemName}/restart${clusterSuffix(envId)}`).then(assertSuccess),
    meta: { success: t('toast.restarted') },
    onSuccess: invalidate,
  });
}

export function useDeleteDeployment() {
  const { t } = useTranslation('kubernetes');
  const envId = useSelectedCluster();
  const invalidate = useSummaryInvalidation();
  return useMutation({
    mutationFn: ({ namespace, name: itemName }: ResourceRef) =>
      api.del(`/kubernetes/deployments/${namespace}/${itemName}`, clusterParams(envId)).then(assertSuccess),
    meta: { success: t('toast.deleted') },
    onSuccess: invalidate,
  });
}

export function useDeletePod() {
  const { t } = useTranslation('kubernetes');
  const envId = useSelectedCluster();
  const invalidate = useSummaryInvalidation();
  return useMutation({
    mutationFn: ({ namespace, name: itemName }: ResourceRef) =>
      api.del(`/kubernetes/pods/${namespace}/${itemName}`, clusterParams(envId)).then(assertSuccess),
    meta: { success: t('toast.deleted') },
    onSuccess: invalidate,
  });
}

export type PodLogsQuery = ResourceRef & {
  container?: string;
  tail: number;
  previous: boolean;
};

/**
 * Reads pod logs as plain text.
 *
 * The endpoint streams `text/plain`, not the usual `{ success, data }` envelope,
 * so this goes through `api.download` (which returns the raw Response) rather
 * than `api.get`. A failure still arrives as the JSON envelope, so the error
 * branch parses it for the translated message.
 */
export async function fetchPodLogs(envId: string, query: PodLogsQuery): Promise<string> {
  const params: Record<string, string> = {
    tail: String(query.tail),
    previous: String(query.previous),
  };
  if (envId) params.cluster = envId;
  if (query.container) params.container = query.container;

  const res = await api.download(`/kubernetes/pods/${query.namespace}/${query.name}/logs`, params);
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ApiResponse | null;
    const envelope = { ...body, error: body?.error ?? `HTTP ${res.status}` };
    const summary = summarizeApiError(envelope);
    throw new ApiError(summary, res.status, {
      summary,
      detail: body?.detail?.trim() || undefined,
      code: body?.code,
      requestId: body?.requestId,
      status: res.status,
    });
  }
  return res.text();
}
