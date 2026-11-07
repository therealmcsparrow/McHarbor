// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { pluginLoader } from '@core/plugins/loader';
import { PLUGIN_QUERY_KEY } from './pluginQueryKey';

/** Must stay in sync with ReturnCheckResult in modules/appstore/plugins/plugin_return.go. */
export type ReturnOutcome = 'clear' | 'blocked' | 'unsigned' | 'no-stable' | 'prerelease-host';

export type ReturnCheck = {
  outcome: ReturnOutcome;
  installedVersion?: string;
  targetVersion?: string;
  blockingMigration?: string;
  /** True when the server sent an outcome this build does not know; it is shown as blocked. */
  unrecognised: boolean;
};

const KNOWN: readonly ReturnOutcome[] = ['clear', 'blocked', 'unsigned', 'no-stable', 'prerelease-host'];

type RawCheck = {
  outcome?: unknown;
  installed_version?: string;
  target_version?: string;
  blocking_migration?: string;
};

// The server's outcome is untrusted input: anything unknown reads as blocked, never as clear.
function normalise(raw: RawCheck): ReturnCheck {
  const known = KNOWN.find((k) => k === raw.outcome);
  return {
    outcome: known ?? 'blocked',
    installedVersion: raw.installed_version,
    targetVersion: raw.target_version,
    blockingMigration: raw.blocking_migration,
    unrecognised: known === undefined,
  };
}

const returnPath = (slug: string) => `/appstore/plugins/${encodeURIComponent(slug)}/return`;

/**
 * usePluginReturnCheck runs the read-only return check once while the dialog is
 * open. Each run downloads the tarball on the server, so nothing refetches on
 * its own: no polling, no focus or reconnect refetch, no retry. Only `refetch`
 * (the operator's "Check again") asks a second time.
 */
export function usePluginReturnCheck(slug: string, enabled: boolean) {
  const query = useQuery({
    queryKey: [PLUGIN_QUERY_KEY, 'return-check', slug],
    queryFn: async () => normalise(assertSuccess(await api.get<RawCheck>(`${returnPath(slug)}/check`))),
    enabled,
    retry: false,
    gcTime: 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    refetchOnMount: false,
  });
  return {
    check: query.data,
    isPending: enabled && (query.isPending || query.isFetching),
    error: query.isError && !query.isFetching ? query.error : null,
    recheck: () => void query.refetch(),
  };
}

type ReturnArgs = { allowUnsigned: boolean };

/**
 * usePluginReturn moves the plugin to stable. `allow_unsigned` is in the body
 * only when the operator ticked the consent box. The server clears the plugin's
 * own prerelease override as part of the return, so the client sends no second request.
 */
export function usePluginReturn(slug: string, pluginName: string) {
  const { t } = useTranslation('plugins');
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ allowUnsigned }: ReturnArgs) => {
      return assertSuccess(
        await api.post<{ slug: string; from: string; to: string }>(
          returnPath(slug),
          allowUnsigned ? { allow_unsigned: true } : {},
        ),
      );
    },
    meta: { success: t('return.success', { name: pluginName }), error: t('return.failed') },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: [PLUGIN_QUERY_KEY] });
      void pluginLoader.refresh();
    },
  });
}
