// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { VolumeInfo } from '@core/types/docker';

export function useVolumes() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['volumes', envId],
    queryFn: () =>
      api
        .get<VolumeInfo[]>('/volumes', envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useCreateVolume() {
  const { t } = useTranslation('volumes');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (params: { name: string; driver?: string }) =>
      api.post(`/volumes${envId ? `?env=${envId}` : ''}`, params).then(assertSuccess),
    meta: { success: t('toast.created') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['volumes'] });
    },
  });
}

export function useRemoveVolume() {
  const { t } = useTranslation('volumes');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (name: string) =>
      api.del(`/volumes/${name}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['volumes'] });
    },
  });
}

export function usePruneVolumes() {
  const { t } = useTranslation('volumes');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: () =>
      api.post(`/volumes/prune${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['volumes'] });
    },
  });
}
