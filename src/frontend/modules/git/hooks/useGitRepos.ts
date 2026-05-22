// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

export type GitRepo = {
  id: string;
  name: string;
  url: string;
  branch: string;
  autoSync: boolean;
  lastSyncAt: string | null;
  lastSyncError: string | null;
  createdAt: string;
};

export function useGitRepos() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['git-repos', envId],
    queryFn: () =>
      api.get<PaginatedData<GitRepo>>('/git', { per_page: '100', ...(envId ? { env: envId } : {}) }).then((r) => r.data?.items ?? []),
    refetchInterval: 30_000,
  });
}

export function useSyncRepo(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post(`/git/${id}/sync`).then(assertSuccess),
    meta: { success: t('git.mutationSynced') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git-repos'] });
    },
  });
}

export function useCreateRepo(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; url: string; branch: string }) =>
      api.post('/git', data).then(assertSuccess),
    meta: { success: t('git.mutationAdded') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git-repos'] });
    },
  });
}

export function useRemoveRepo(t: (key: string) => string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/git/${id}`).then(assertSuccess),
    meta: { success: t('git.mutationRemoved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git-repos'] });
    },
  });
}

export const STATUS_VARIANT = (repo: GitRepo): 'success' | 'destructive' | 'secondary' => {
  if (repo.lastSyncError) return 'destructive';
  if (repo.lastSyncAt) return 'success';
  return 'secondary';
};
