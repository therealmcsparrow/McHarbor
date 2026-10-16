// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { ContainerInspect } from '@core/types/docker';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useOnCallQuery } from '@resources/hooks/useOnCallQuery';

export function useOnCallContainer(envId: string, containerId: string) {
  return useOnCallQuery<ContainerInspect>(
    ['container', envId, containerId],
    `/containers/${encodeURIComponent(containerId)}`,
    { env: envId },
    { enabled: Boolean(envId && containerId), refetchInterval: 15_000 },
  );
}

export type RestartVariables = { envId: string; containerId: string };

export function useRestartContainer() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  return useMutation({
    mutationFn: ({ envId, containerId }: RestartVariables) =>
      api
        .post(`/containers/${encodeURIComponent(containerId)}/restart?env=${encodeURIComponent(envId)}`)
        .then(assertSuccess),
    meta: { success: t('toast.restarted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['oncall', 'container'] });
      void queryClient.invalidateQueries({ queryKey: ['oncall', 'containers'] });
    },
  });
}
