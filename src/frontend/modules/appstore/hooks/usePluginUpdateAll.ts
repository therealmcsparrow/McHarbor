// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { pluginLoader } from '@core/plugins/loader';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import type { PluginEntry } from '../types/plugins';
import { streamPluginUpdate } from './streamPluginUpdate';

/**
 * Updates plugins through the shared batch progress dialog, the same one the
 * container and stack updates use. Strictly sequential: an update uninstalls
 * before it reinstalls and the host reloads its plugin runtime afterwards, so
 * two in flight would corrupt each other. A failed plugin is recorded on its
 * row and the run carries on with the next one.
 */
export function usePluginUpdateAll() {
  const { t } = useTranslation('plugins');
  const queryClient = useQueryClient();
  const batch = useBatchProgressOperation();
  // State lags a double click by a render; the ref closes that window.
  const startedRef = useRef(false);

  async function updateAll(entries: PluginEntry[]) {
    if (entries.length === 0 || startedRef.current) return;
    startedRef.current = true;
    try {
      await batch.runBatchOperation<PluginEntry>({
        title: t('store.updateAllDialogTitle'),
        description: t('store.updateAllDialogDescription'),
        actionLabel: t('store.updateAllActionLabel'),
        items: entries,
        getKey: (entry) => entry.slug,
        getLabel: (entry) => entry.name,
        execute: async (entry, context) => {
          await streamPluginUpdate(
            entry.slug,
            { network: t('install.network_error'), connect: t('install.connect_failed') },
            (evt) => {
              if (evt.message) context.setDetail(evt.message);
            },
          );
        },
        // The sidebar, routes and widgets read the loader store, not the query
        // cache, so one refresh after the run lands every updated plugin.
        onComplete: async () => {
          await queryClient.invalidateQueries({ queryKey: ['appstore-plugins'] });
          await queryClient.invalidateQueries({ queryKey: ['auth', 'me', 'permissions'] });
          await pluginLoader.refresh({ force: true });
        },
      });
    } finally {
      startedRef.current = false;
    }
  }

  return {
    updateAll,
    dialogState: batch.dialogState,
    closeDialog: batch.closeDialog,
    isRunning: batch.isRunning,
  };
}
