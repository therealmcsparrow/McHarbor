// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type VapidRegenerateResult = {
  publicKey: string;
  removedSubscriptions: number;
};

/**
 * useRegenerateVapidKey replaces the instance-wide VAPID key pair. Every
 * stored device subscription was made against the old public key, so the
 * server deletes them all and each device must subscribe again.
 */
export function useRegenerateVapidKey() {
  const { t } = useTranslation('settings');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => assertSuccess(await api.post<VapidRegenerateResult>('/push/vapid/regenerate')),
    meta: {
      success: (data: unknown) =>
        t('notifications.pushRegenerated', {
          count: (data as VapidRegenerateResult).removedSubscriptions,
        }),
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['oncall', 'push'] }),
  });
}
