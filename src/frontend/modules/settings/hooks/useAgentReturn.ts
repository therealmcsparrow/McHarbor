// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { AGENT_INVENTORY_KEY } from '@resources/hooks/useAgentInventory';
import { assertSuccess } from '@resources/utils/api-mutation';
import { updatesKeys } from './queryKeys';

function useRefreshAgents() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: AGENT_INVENTORY_KEY });
    void qc.invalidateQueries({ queryKey: updatesKeys.selfSettings });
  };
}

/**
 * useReturnAgent starts moving one agent to stable and pins it. The pin is written
 * before the move, so the inventory is refreshed on failure too: the row must
 * show the pin and the reason, not the state from before the click.
 */
export function useReturnAgent() {
  const { t } = useTranslation('settings');
  const refresh = useRefreshAgents();
  return useMutation({
    mutationFn: async (envId: string) =>
      assertSuccess(await api.post<unknown>(`/updates/agents/${encodeURIComponent(envId)}/return`)),
    // A 200 means the pin is written and the move has started, not that it has finished.
    meta: { success: t('prerelease.return.started') },
    onSettled: refresh,
  });
}

/** useReleasePin removes one agent's pin; the server drops only an exclusion the pin itself added. */
export function useReleasePin() {
  const { t } = useTranslation('settings');
  const refresh = useRefreshAgents();
  return useMutation({
    mutationFn: async (envId: string) =>
      assertSuccess(await api.del<unknown>(`/updates/agents/${encodeURIComponent(envId)}/pin`)),
    meta: { success: t('prerelease.pin.released'), error: t('prerelease.pin.releaseFailed') },
    onSettled: refresh,
  });
}
