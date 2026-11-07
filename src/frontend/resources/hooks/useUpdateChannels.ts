// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Must stay in sync with updatechannel.Settings in src/backend/core/updatechannel/settings.go. */
export type UpdateChannels = {
  mcharborPrereleases: boolean;
  agentPrereleases: boolean;
  pluginPrereleases: boolean;
  mcharborAutoPrereleases: boolean;
  agentAutoPrereleases: boolean;
};

export type UpdateChannelPatch = Partial<UpdateChannels>;

export const UPDATE_CHANNELS_KEY = ['updates', 'channels'] as const;

/**
 * useUpdateChannels reads the prerelease switches. Without `updates.view` it
 * never asks, so a denied reader gets `canView: false` and an inert query,
 * not a 403 and not a page error.
 */
export function useUpdateChannels() {
  const canView = usePermission(PERM.UPDATES_VIEW);
  const query = useQuery({
    queryKey: UPDATE_CHANNELS_KEY,
    queryFn: async () => assertSuccess(await api.get<UpdateChannels>('/updates/channels')),
    enabled: canView,
  });
  return {
    canView,
    settings: query.data,
    isLoading: canView && query.isLoading,
    isError: query.isError,
    refetch: query.refetch,
  };
}

/**
 * useSetUpdateChannel saves one switch. The body carries only the fields given,
 * so a Store page holding stale values cannot overwrite the McHarbor or agent
 * switches another tab changed.
 */
export function useSetUpdateChannel() {
  const { t } = useTranslation('plugins');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (patch: UpdateChannelPatch) =>
      assertSuccess(await api.put<UpdateChannels>('/updates/channels', patch)),
    meta: { error: t('channels.saveFailed') },
    onSuccess: (settings) => {
      qc.setQueryData(UPDATE_CHANNELS_KEY, settings);
      void qc.invalidateQueries({ queryKey: UPDATE_CHANNELS_KEY });
    },
  });
}
