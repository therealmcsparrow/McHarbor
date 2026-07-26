// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useTranslation } from 'react-i18next';
import type {
  ApprovalInput,
  CreatePipelineInput,
  GitOpsApproval,
  GitOpsPipeline,
  GitOpsPRPreview,
  GitOpsPromotion,
  PromoteInput,
  UpdatePipelineInput,
} from '../types';

export function useGitOpsPipelines() {
  return useQuery({
    queryKey: ['gitops-pipelines'],
    queryFn: () =>
      api
        .get<GitOpsPipeline[]>('/gitops/pipelines')
        .then((r) => r.data ?? []),
    refetchInterval: 15_000,
  });
}

export function useGitOpsPipeline(id: string | undefined) {
  return useQuery({
    queryKey: ['gitops-pipeline', id],
    queryFn: () =>
      api
        .get<GitOpsPipeline>(`/gitops/pipelines/${id}`)
        .then((r) => r.data),
    enabled: Boolean(id),
    refetchInterval: 10_000,
  });
}

export function useGitOpsPromotions(pipelineId: string | undefined) {
  return useQuery({
    queryKey: ['gitops-promotions', pipelineId],
    queryFn: () =>
      api
        .get<PaginatedData<GitOpsPromotion>>(`/gitops/pipelines/${pipelineId}/promotions`, {
          per_page: '100',
        })
        .then((r) => r.data?.items ?? []),
    enabled: Boolean(pipelineId),
    refetchInterval: 10_000,
  });
}

export function useGitOpsApprovals() {
  return useQuery({
    queryKey: ['gitops-approvals'],
    queryFn: () =>
      api
        .get<GitOpsApproval[]>('/gitops/approvals')
        .then((r) => r.data ?? []),
    refetchInterval: 15_000,
  });
}

export function useGitOpsPRPreviews() {
  return useQuery({
    queryKey: ['gitops-previews'],
    queryFn: () =>
      api
        .get<GitOpsPRPreview[]>('/gitops/previews')
        .then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useCreatePipeline(t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreatePipelineInput) =>
      api.post<GitOpsPipeline>('/gitops/pipelines', input).then(assertSuccess),
    meta: { success: t('gitops.mutationCreated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gitops-pipelines'] }),
  });
}

export function useUpdatePipeline(t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdatePipelineInput }) =>
      api.put<GitOpsPipeline>(`/gitops/pipelines/${id}`, input).then(assertSuccess),
    meta: { success: t('gitops.mutationUpdated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gitops-pipelines'] }),
  });
}

export function useDeletePipeline(t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/gitops/pipelines/${id}`).then(assertSuccess),
    meta: { success: t('gitops.mutationDeleted') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['gitops-pipelines'] }),
  });
}

export function usePromoteStage(t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ pipelineId, input }: { pipelineId: string; input: PromoteInput }) =>
      api.post<GitOpsPromotion>(`/gitops/pipelines/${pipelineId}/promote`, input).then(assertSuccess),
    meta: { success: t('gitops.mutationPromoted') },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['gitops-pipelines'] });
      qc.invalidateQueries({ queryKey: ['gitops-promotions'] });
      qc.invalidateQueries({ queryKey: ['gitops-approvals'] });
    },
  });
}

export function useResolveApproval(t: (key: string) => string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, action, input }: { id: string; action: 'approve' | 'reject'; input: ApprovalInput }) =>
      api.post<GitOpsApproval>(`/gitops/approvals/${id}/${action}`, input).then(assertSuccess),
    meta: { success: t('gitops.mutationResolved') },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['gitops-approvals'] });
      qc.invalidateQueries({ queryKey: ['gitops-pipelines'] });
      qc.invalidateQueries({ queryKey: ['gitops-promotions'] });
    },
  });
}

export function useTranslationOrThrow() {
  // Helper to keep the t reference typed (workaround for tsc scope).
  return useTranslation('gitops');
}
