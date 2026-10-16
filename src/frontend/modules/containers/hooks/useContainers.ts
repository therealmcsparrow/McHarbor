// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { ContainerInspect } from '@core/types/docker';
export { useContainersBulkStats } from '@resources/hooks/useContainersBulkStats';
export { useContainers } from '@resources/hooks/useContainers';
export * from './useMoveContainer';

export function useContainer(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container', envId, id],
    queryFn: () => api.get<ContainerInspect>(`/containers/${id}`, envId ? { env: envId } : {}).then(assertSuccess),
    enabled: !!id,
  });
}

/** Variables accepted by `useContainerAction`. The container ID plus
 * the action name (start, stop, restart, pause, unpause, kill). */
export type ContainerActionVariables = {
  id: string;
  action: string;
};

export const CONTAINER_ACTION_MUTATION_KEY = ['containers', 'action'] as const;

export function useContainerAction() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationKey: CONTAINER_ACTION_MUTATION_KEY,
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      api.post(`/containers/${id}/${action}${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: {
      success: (_: unknown, vars: unknown) => {
        const { action } = vars as { action: string };
        const labels: Record<string, string> = {
          start: t('toast.started'),
          stop: t('toast.stopped'),
          restart: t('toast.restarted'),
          pause: t('toast.paused'),
          unpause: t('toast.resumed'),
          kill: t('toast.killed'),
        };
        return labels[action] ?? t('actions.actionCompleted');
      },
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['container'] });
    },
  });
}

export function useRenameContainer() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, name: newName, unlockProtected }: { id: string; name: string; unlockProtected?: boolean }) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post(`/containers/${id}/rename${envQuery}`, { name: newName, unlockProtected }).then(assertSuccess);
    },
    meta: { success: t('toast.renamed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['container'] });
    },
  });
}

type RemoveContainerOptions = {
  id: string;
  force: boolean;
  removeVolumes: boolean;
  removeImage: boolean;
  removeStack: boolean;
  removeNetwork?: boolean;
};

export function useRemoveContainer() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, ...body }: RemoveContainerOptions) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post(`/containers/${id}/remove${envQuery}`, body).then(assertSuccess);
    },
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      void queryClient.invalidateQueries({ queryKey: ['stacks'] });
      void queryClient.invalidateQueries({ queryKey: ['images'] });
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}

export function usePruneContainers() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: () => api.post(`/containers/prune${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });
}
