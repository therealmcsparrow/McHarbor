// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { Snapshot, VM } from '../types';
import { invalidateVM } from './vmQueryKeys';

export function useCreateSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      name: string;
      description?: string;
      withMemory?: boolean;
      pauseGuest?: boolean;
    }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Snapshot>(`/vms/${id}/snapshots`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useRevertSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; snapshotId: string; stopFirst?: boolean; restartAfter?: boolean }) =>
      assertSuccess(
        await api.post<null>(`/vms/${input.id}/snapshots/${input.snapshotId}/revert`, {
          stopFirst: input.stopFirst ?? false,
          restartAfter: input.restartAfter ?? false,
        }),
      ),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useDeleteSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; snapshotId: string }) =>
      assertSuccess(await api.del<null>(`/vms/${input.id}/snapshots/${input.snapshotId}`)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useCloneVM() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('vm');
  return useMutation({
    meta: { success: t('msg.vm.cloned') },
    mutationFn: async (input: { id: string; name: string; linked: boolean }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<VM>(`/vms/${id}/clone`, body));
    },
    onSuccess: () => invalidateVM(queryClient),
  });
}

export function useSetTemplate() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('vm');
  return useMutation({
    meta: { success: t('msg.vm.templated') },
    mutationFn: async (input: { id: string; isTemplate: boolean }) =>
      assertSuccess(await api.post<null>(`/vms/${input.id}/template`, { isTemplate: input.isTemplate })),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

/**
 * A clone's copy progress while the clone request is still open.
 *
 * The clone's id only arrives with the response, but its row exists from the
 * start and carries `cloneProgress`, so the row is found by the name the
 * operator typed — unique per host, which the server enforces.
 */
export function useCloneProgress(environmentId: string, cloneName: string, active: boolean) {
  return useQuery({
    queryKey: ['vms', 'clone-progress', environmentId, cloneName],
    enabled: active && Boolean(cloneName),
    refetchInterval: active ? 1000 : false,
    queryFn: async () => assertSuccess(await api.get<VM[]>('/vms', { env: environmentId })) ?? [],
    select: (vms: VM[]) => vms.find((entry) => entry.name === cloneName)?.cloneProgress,
  });
}
