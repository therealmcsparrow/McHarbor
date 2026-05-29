// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { Workflow } from '../types';

export type WorkflowRun = {
  id: string;
  workflowId: string;
  status: string;
  trigger: string;
  durationMs: number;
  nodeCount: number;
  error: string;
  startedAt: string;
  finishedAt: string;
};

export function useWorkflows() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['workflows', envId],
    queryFn: () => api.get<PaginatedData<Workflow>>('/workflows', { per_page: '100', ...(envId ? { env: envId } : {}) }).then((r) => r.data?.items ?? []),
  });
}

export function useWorkflow(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['workflows', envId, id],
    queryFn: () => api.get<Workflow>(`/workflows/${id}`, envId ? { env: envId } : {}).then((r) => r.data),
    enabled: !!id,
  });
}

export function useWorkflowRuns(workflowId?: string) {
  const envId = useEnvironmentStore((state) => state.currentId);

  return useQuery({
    queryKey: ['workflow-runs', envId, workflowId],
    queryFn: () => {
      const params: Record<string, string> = { per_page: '100' };
      if (workflowId) params.workflow_id = workflowId;
      if (envId) params.env = envId;
      return api
        .get<PaginatedData<WorkflowRun>>('/workflows/runs', params)
        .then((response) => response.data?.items ?? []);
    },
    refetchInterval: 10_000,
  });
}

export function useCreateWorkflow() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; description?: string }) => api.post<Workflow>('/workflows', data).then(assertSuccess),
    meta: { success: t('workflows.mutationCreated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['workflows'] }),
  });
}

export function useUpdateWorkflow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string; name?: string; description?: string; status?: string; canvasData?: string }) =>
      api.put<Workflow>(`/workflows/${id}`, data).then(assertSuccess),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: ['workflows'] });
      queryClient.invalidateQueries({ queryKey: ['workflows', vars.id] });
    },
  });
}

export function useDeleteWorkflow() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/workflows/${id}`).then(assertSuccess),
    meta: { success: t('workflows.mutationDeleted') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['workflows'] }),
  });
}

function currentLanguage() {
  const stored = localStorage.getItem('mcharbor-language');
  if (!stored) return 'en';
  try {
    return JSON.parse(stored)?.state?.language || 'en';
  } catch {
    return stored;
  }
}

function filenameFromDisposition(disposition: string | null, fallback: string) {
  const match = disposition?.match(/filename="([^"]+)"/);
  return match?.[1] || fallback;
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function useExportWorkflow() {
  const { t } = useTranslation('common');
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) => {
      const params = envId ? `?${new URLSearchParams({ env: envId }).toString()}` : '';
      const response = await fetch(`/api/workflows/${id}/export${params}`, {
        credentials: 'include',
        headers: { 'Accept-Language': currentLanguage() },
      });
      if (!response.ok) {
        throw new Error(`export failed with status ${response.status}`);
      }
      const blob = await response.blob();
      const fallback = `${name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'workflow'}.mcharbor-workflow.json`;
      saveBlob(blob, filenameFromDisposition(response.headers.get('content-disposition'), fallback));
    },
    meta: {
      success: t('workflows.mutationExported'),
      error: t('workflows.mutationExportFailed'),
    },
  });
}

export function useImportWorkflow() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (file: File) => {
      const text = await file.text();
      const data = JSON.parse(text) as unknown;
      return api.post<Workflow>('/workflows/import', data).then(assertSuccess);
    },
    meta: {
      success: t('workflows.mutationImported'),
      error: t('workflows.mutationImportFailed'),
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['workflows'] }),
  });
}

