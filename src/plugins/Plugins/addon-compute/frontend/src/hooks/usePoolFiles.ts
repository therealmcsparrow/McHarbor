// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { apiErrorOf } from '@resources/utils/api-mutation';
import { hostToast } from '../hostToast';
import type { PoolFile } from '../types';

const KEYS = {
  listing: (poolId: string, path: string) => ['compute', 'pool-files', poolId, path] as const,
  content: (poolId: string, path: string) => ['compute', 'pool-file', poolId, path] as const,
};

async function expectOk(res: ApiResponse<unknown>): Promise<void> {
  if (!res.success) throw apiErrorOf(res);
}

export function usePoolFiles(poolId: string, path: string, enabled = true) {
  return useQuery({
    queryKey: KEYS.listing(poolId, path),
    enabled: enabled && Boolean(poolId),
    queryFn: async () => {
      const res = await api.get<PoolFile[]>(`/compute/pools/${poolId}/files`, {
        path,
      });
      if (!res.success) throw apiErrorOf(res);
      return res.data ?? [];
    },
  });
}

/**
 * A file's text, fetched outside the api client because the route answers with
 * the bytes themselves rather than the `{ success, data }` envelope. Anything
 * over the agent's inline limit is refused there, so this never streams.
 */
export function usePoolFileContent(poolId: string, path: string, enabled = true) {
  return useQuery({
    queryKey: KEYS.content(poolId, path),
    enabled: enabled && Boolean(poolId) && Boolean(path),
    queryFn: async () => {
      const query = new URLSearchParams({ path });
      const res = await fetch(`/api/compute/pools/${poolId}/files/content?${query}`, {
        credentials: 'include',
      });
      if (!res.ok) throw new Error(String(res.status));
      return res.text();
    },
  });
}

export function useSavePoolFile(poolId: string) {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');

  return useMutation({
    mutationFn: async (input: { path: string; content: string }) =>
      expectOk(await api.put(`/compute/pools/${poolId}/files/content`, input)),
    onSuccess: (_result, input) => {
      hostToast.success(t('pools.files.toast.saved'));
      void queryClient.invalidateQueries({
        queryKey: KEYS.content(poolId, input.path),
      });
      void queryClient.invalidateQueries({
        queryKey: ['compute', 'pool-files', poolId],
      });
    },
  });
}

export function useCreatePoolDir(poolId: string) {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');

  return useMutation({
    mutationFn: async (input: { path: string }) =>
      expectOk(await api.post(`/compute/pools/${poolId}/files/mkdir`, input)),
    onSuccess: () => {
      hostToast.success(t('pools.files.toast.created'));
      void queryClient.invalidateQueries({
        queryKey: ['compute', 'pool-files', poolId],
      });
    },
  });
}

export function useRenamePoolFile(poolId: string) {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');

  return useMutation({
    mutationFn: async (input: { path: string; newName: string }) =>
      expectOk(await api.post(`/compute/pools/${poolId}/files/rename`, input)),
    onSuccess: () => {
      hostToast.success(t('pools.files.toast.renamed'));
      void queryClient.invalidateQueries({
        queryKey: ['compute', 'pool-files', poolId],
      });
    },
  });
}

export function useDeletePoolFile(poolId: string) {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');

  return useMutation({
    mutationFn: async (input: { path: string; recursive?: boolean }) =>
      expectOk(
        await api.del(`/compute/pools/${poolId}/files`, {
          path: input.path,
          ...(input.recursive ? { recursive: 'true' } : {}),
        }),
      ),
    onSuccess: () => {
      hostToast.success(t('pools.files.toast.deleted'));
      void queryClient.invalidateQueries({
        queryKey: ['compute', 'pool-files', poolId],
      });
    },
  });
}
