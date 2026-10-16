// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
import { useCallback, useMemo } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useAllContainerBackupPlans, useAllContainerBackupRuns } from '@resources/hooks/useContainerBackups';
import { useBackupProviderPlans, useBackupProviderRuns } from '@resources/hooks/useBackupProviderRuns';
import { mergeBackupRunRows } from '../pages/backup-run-rows';
import { mergeBackupPlanRows } from '../pages/backup-plan-rows';
import type { BackupsTabId } from '../components/backups-tab-groups';

/** Loads and merges the backup runs and plans (host and provider) for one environment scope. */
export function useBackupsPageData(scopeEnvId: string) {
  const queryClient = useQueryClient();
  const environments = useEnvironmentStore((s) => s.environments);
  const runsQuery = useAllContainerBackupRuns(scopeEnvId);
  const providerRunsQuery = useBackupProviderRuns(scopeEnvId);
  const plansQuery = useAllContainerBackupPlans(scopeEnvId);
  const providerPlansQuery = useBackupProviderPlans(scopeEnvId);

  const reloadTab = useCallback(
    (tabId: BackupsTabId) => {
      if (tabId === 'runs' || tabId === 'overview') {
        void queryClient.invalidateQueries({
          queryKey: ['container-backup-runs'],
        });
        void queryClient.invalidateQueries({
          queryKey: ['backup-provider-runs'],
        });
      }
      if (tabId === 'plans' || tabId === 'overview') {
        void queryClient.invalidateQueries({
          queryKey: ['container-backup-plans'],
        });
        void queryClient.invalidateQueries({
          queryKey: ['backup-provider-plans'],
        });
      }
    },
    [queryClient],
  );

  const runRows = useMemo(
    () => mergeBackupRunRows(runsQuery.data, providerRunsQuery.data?.providerRuns ?? []),
    [providerRunsQuery.data, runsQuery.data],
  );

  const planRows = useMemo(
    () => mergeBackupPlanRows(plansQuery.data, providerPlansQuery.data?.providerPlans ?? []),
    [plansQuery.data, providerPlansQuery.data],
  );

  const envNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const env of environments) {
      map.set(env.id, env.name);
    }
    return map;
  }, [environments]);

  return {
    runRows,
    planRows,
    envNameById,
    reloadTab,
    runsLoading: runsQuery.isLoading,
    plansLoading: plansQuery.isLoading,
    overviewLoading:
      (runsQuery.isLoading || providerRunsQuery.isLoading) && (plansQuery.isLoading || providerPlansQuery.isLoading),
  };
}
