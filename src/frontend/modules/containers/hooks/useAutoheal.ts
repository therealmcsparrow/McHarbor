// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * Per-container auto-heal preference.
 *
 * The healing loop itself has been running in `modules/autoheal/runtime.go`
 * all along; what was missing was any way to see or change the per-container
 * switch it reads. These two hooks are that control surface.
 *
 * Both routes accept `containers.manage` **or** `autoheal.manage`
 * (`RequireAnyPermission`), so the UI gate is any-of rather than a single key —
 * a container manager can reach them without holding an autoheal key at all.
 */
export type AutohealPreference = {
  containerId: string;
  containerName: string;
  enabled: boolean;
  lastHealAt?: string;
  restartCount: number;
  wasEverHealthy: boolean;
};

export const AUTOHEAL_PREFERENCE_KEY = 'autoheal-preference';

export function useAutohealPreference(containerId: string, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);

  return useQuery({
    queryKey: [AUTOHEAL_PREFERENCE_KEY, envId, containerId],
    queryFn: () =>
      api
        .get<AutohealPreference>(`/autoheal/preference/${containerId}`, envId ? { env: envId } : {})
        .then(assertSuccess),
    enabled: enabled && !!containerId,
  });
}

export function useSetAutohealPreference(containerId: string) {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: (nextEnabled: boolean) =>
      api
        .post<AutohealPreference>(
          `/autoheal/preference/${containerId}${envId ? `?env=${encodeURIComponent(envId)}` : ''}`,
          { enabled: nextEnabled },
        )
        .then(assertSuccess),
    meta: { success: t('toast.autohealUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: [AUTOHEAL_PREFERENCE_KEY, envId, containerId],
      });
    },
  });
}
