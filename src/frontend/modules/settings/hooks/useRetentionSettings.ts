// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { RetentionSettingsData } from '../types';

export function useRetentionSettings() {
  return useQuery({
    queryKey: ['settings', 'retention'],
    queryFn: () => api.get<RetentionSettingsData>('/settings/retention').then((r) => r.data),
  });
}

export function useSaveRetentionSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: RetentionSettingsData) =>
      api.put('/settings/retention', data).then(assertSuccess),
    meta: { success: t('toast.retentionUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'retention'] });
    },
  });
}
