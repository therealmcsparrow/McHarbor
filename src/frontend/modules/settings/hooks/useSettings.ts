// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AppSettings } from '../types';

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get<AppSettings>('/settings').then((r) => r.data),
  });
}

export function useSaveSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (settings: Partial<AppSettings>) =>
      api.put('/settings', settings).then(assertSuccess),
    meta: { success: t('toast.settingsSaved') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
}
