// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { HostMetrics, PruneRequest, PruneResult } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';

export function useHostMetrics() {
  const envId = useEnvironmentStore((state) => state.currentId);
  const uploadActive = useEnvironmentUploadActive(envId);

  return useQuery({
    queryKey: ['host-metrics', envId],
    queryFn: () =>
      api
        .get<HostMetrics>('/metrics/host', envId ? { env: envId } : {})
        .then((response) => response.data),
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
      return api
        .post<PruneResult>(`/metrics/host/prune${envQuery}`, req)
        .then((r) => r.data as PruneResult);
    },
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['host-metrics'] });
      queryClient.invalidateQueries({ queryKey: ['containers'] });
      queryClient.invalidateQueries({ queryKey: ['images'] });
      queryClient.invalidateQueries({ queryKey: ['volumes'] });
      queryClient.invalidateQueries({ queryKey: ['networks'] });
    },
  });
}
