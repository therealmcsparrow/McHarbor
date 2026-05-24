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

export function useContainer(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container', envId, id],
    queryFn: () =>
      api
        .get<ContainerInspect>(`/containers/${id}`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!id,
  });
}

export function useContainerAction() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      api.post(`/containers/${id}/${action}${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: {
      success: (_: unknown, vars: unknown) => {
        const { action } = vars as { action: string };
        const labels: Record<string, string> = {
          start: t('toast.started'), stop: t('toast.stopped'), restart: t('toast.restarted'),
          pause: t('toast.paused'), unpause: t('toast.resumed'), kill: t('toast.killed'),
        };
        return labels[action] ?? t('actions.actionCompleted');
      },
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
    },
  });
}

type RemoveContainerOptions = {
  id: string;
  force: boolean;
  removeVolumes: boolean;
  removeImage: boolean;
  removeStack: boolean;
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
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['container'] });
      queryClient.invalidateQueries({ queryKey: ['stacks'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

export function usePruneContainers() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: () =>
      api.post(`/containers/prune${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });
}
