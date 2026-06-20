// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { AutohealPreference } from '@core/types/docker';
import { useEnvironmentStore } from '@resources/stores/environment';

export function useAutohealPreference(containerId: string | null | undefined) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['autoheal', envId, containerId],
    queryFn: () =>
      api
        .get<AutohealPreference>(
          `/autoheal/preference/${containerId}`,
          envId ? { env: envId } : {},
        )
        .then((r) => r.data),
    enabled: !!containerId,
    refetchInterval: 10_000,
  });
}

export function useSetAutohealPreference(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (enabled: boolean) => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api
        .post<AutohealPreference>(`/autoheal/preference/${containerId}${envQuery}`, { enabled })
        .then((r) => r.data as AutohealPreference);
    },
    meta: { success: t('toast.autohealUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['autoheal'] });
    },
  });
}
