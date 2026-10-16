// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerInspect } from '@core/types/docker';

type RecreateResult = {
  Id: string;
  confirmed?: boolean;
};

const RECREATE_CONFIRM_ATTEMPTS = 30;

async function confirmReplacement(id: string, envId: string | null): Promise<boolean> {
  for (let attempt = 0; attempt < RECREATE_CONFIRM_ATTEMPTS; attempt++) {
    const response = await api.get<ContainerInspect>(
      `/containers/${encodeURIComponent(id)}`,
      envId ? { env: envId } : {},
    );
    if (response.success && response.data?.Id === id && response.data.State?.Running) {
      return true;
    }
    await new Promise((resolve) => setTimeout(resolve, 1_000));
  }
  return false;
}

/** Owns the update and recreate mutations the container edit flow drives. */
export function useContainerEditMutations(container: ContainerInspect | undefined, envId: string | null) {
  const { t } = useTranslation('containers');
  const queryClient = useQueryClient();
  const envQuery = envId ? `?env=${envId}` : '';

  const updateMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      api.post(`/containers/${container?.Id}/update${envQuery}`, payload).then(assertSuccess),
    meta: { success: t('toast.updated'), error: t('toast.updateFailed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });

  const recreateMutation = useMutation({
    mutationFn: async (payload: Record<string, unknown>) => {
      const recreated = await api
        .post<RecreateResult>(`/containers/${container?.Id}/recreate${envQuery}`, payload)
        .then(assertSuccess);
      if (!recreated.Id || recreated.Id === container?.Id) return recreated;
      return {
        ...recreated,
        confirmed: await confirmReplacement(recreated.Id, envId),
      };
    },
    meta: { success: t('toast.recreated'), error: t('toast.updateFailed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['container'] });
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });

  return { updateMutation, recreateMutation };
}
