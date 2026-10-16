// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type BackupProviderRun = {
  id: string;
  sourceKind: string;
  sourceSlug?: string;
  sourceLabel?: string;
  subjectId?: string;
  subjectName?: string;
  environmentId?: string;
  operation: string;
  status: string;
  kind?: string;
  sizeBytes: number;
  destinations?: string[];
  error?: string;
  warning?: string;
  startedAt: string;
  completedAt?: string;
  durationMs: number;
};

export type BackupSourceInfo = {
  kind: string;
  slug?: string;
  displayName: string;
  icon?: string;
  canRestore: boolean;
  canDelete: boolean;
  available: boolean;
  error?: string;
};

export type BackupProviderRunsPage = {
  providerRuns: BackupProviderRun[];
  sources: BackupSourceInfo[];
};

export function useBackupProviderRuns(envId: string | undefined | null) {
  return useQuery({
    queryKey: ['backup-provider-runs', envId ?? 'all'],
    queryFn: () =>
      api
        .get<BackupProviderRunsPage>('/container-backups/provider-runs', envId ? { env: envId } : undefined)
        .then(assertSuccess),
    staleTime: 10_000,
    refetchInterval: (query) =>
      (query.state.data?.providerRuns ?? []).some((run) => run.status === 'running') ? 5000 : false,
  });
}

export type BackupProviderPlan = {
  id: string;
  sourceKind: string;
  sourceSlug?: string;
  sourceLabel?: string;
  subjectId?: string;
  subjectName?: string;
  environmentId?: string;
  name: string;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  lastRunAt?: string;
  nextRunAt?: string;
};

export type BackupProviderPlansPage = {
  providerPlans: BackupProviderPlan[];
  sources: BackupSourceInfo[];
};

export function useBackupProviderPlans(envId: string | undefined | null) {
  return useQuery({
    queryKey: ['backup-provider-plans', envId ?? 'all'],
    queryFn: () =>
      api
        .get<BackupProviderPlansPage>('/container-backups/provider-plans', envId ? { env: envId } : undefined)
        .then(assertSuccess),
    staleTime: 30_000,
  });
}
