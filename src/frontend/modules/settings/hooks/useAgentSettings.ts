// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AgentSettingsData } from '../types';

export function useAgentSettings() {
  return useQuery({
    queryKey: ['settings', 'agent'],
    queryFn: () => api.get<AgentSettingsData>('/settings/agent').then((r) => r.data),
  });
}

export function useSaveAgentSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (data: AgentSettingsData) =>
      api.put('/settings/agent', data).then(assertSuccess),
    meta: { success: t('toast.agentUpdated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'agent'] });
    },
  });
}
