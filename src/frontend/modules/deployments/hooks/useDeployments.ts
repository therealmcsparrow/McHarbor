// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { DeploymentSummary, DeploymentDetail } from '@core/types/kubernetes';

export function useDeployments(namespace = '') {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['deployments', envId, namespace],
    queryFn: () =>
      api
        .get<DeploymentSummary[]>('/deployments', {
          ...(envId ? { env: envId } : {}),
          ...(namespace ? { namespace } : {}),
        })
        .then((r) => r.data ?? []),
    refetchInterval: 10_000,
  });
}

export function useDeployment(namespace: string, name: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['deployment', envId, namespace, name],
    queryFn: () =>
      api
        .get<DeploymentDetail>(`/deployments/${namespace}/${name}`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!namespace && !!name,
  });
}

export function useScaleDeployment() {
  const { t } = useTranslation('kubernetes');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ namespace, name, replicas }: { namespace: string; name: string; replicas: number }) =>
      api.post(`/deployments/${namespace}/${name}/scale${envId ? `?env=${envId}` : ''}`, {
        replicas,
      }).then(assertSuccess),
    meta: { success: t('deployments.toast.scaled') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployments'] });
      queryClient.invalidateQueries({ queryKey: ['deployment'] });
    },
  });
}

export function useRestartDeployment() {
  const { t } = useTranslation('kubernetes');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: ({ namespace, name }: { namespace: string; name: string }) =>
      api.post(`/deployments/${namespace}/${name}/restart${envId ? `?env=${envId}` : ''}`).then(assertSuccess),
    meta: { success: t('deployments.toast.restarted') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployments'] });
      queryClient.invalidateQueries({ queryKey: ['deployment'] });
    },
  });
}
