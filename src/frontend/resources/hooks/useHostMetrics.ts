// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { HostMetrics, PruneRequest, PruneResult } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';

import { assertSuccess } from '@resources/utils/api-mutation';

export function useHostMetrics() {
  const envId = useEnvironmentStore((state) => state.currentId);
  const uploadActive = useEnvironmentUploadActive(envId);

  return useQuery({
    queryKey: ['host-metrics', envId],
    queryFn: () => api.get<HostMetrics>('/metrics/host', envId ? { env: envId } : {}).then(assertSuccess),
    refetchInterval: uploadActive ? false : 5_000,
    enabled: !uploadActive,
  });
}

export function useHostPrune() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((state) => state.currentId);
  const { t } = useTranslation('host');

  return useMutation({
    mutationFn: (req: PruneRequest) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api.post<PruneResult>(`/metrics/host/prune${envQuery}`, req).then((r) => r.data as PruneResult);
    },
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['host-metrics'] });
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
      void queryClient.invalidateQueries({ queryKey: ['images'] });
      void queryClient.invalidateQueries({ queryKey: ['volumes'] });
      void queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}
