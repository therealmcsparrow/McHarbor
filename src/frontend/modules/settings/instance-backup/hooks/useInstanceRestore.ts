// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { refusalOf, type ExportManifest } from './useInstanceBackups';

export type RestoreMode = 'full' | 'migration' | 'selective';
export type Conflict = 'replace' | 'keep';

export const RESTORE_CATEGORIES = [
  'access',
  'environments',
  'automation',
  'stacks',
  'secrets',
  'settings',
  'plugins',
  'other',
] as const;
export type RestoreCategory = (typeof RESTORE_CATEGORIES)[number] | 'audit';

export type RestoreSource = { exportId?: string; uploadId?: string };

export type RestoreInput = {
  source: RestoreSource;
  passphrase?: string;
  keyFile?: string;
  mode: RestoreMode;
  categories?: string[];
  includeAudit: boolean;
  conflict?: Conflict;
};

export type TablePlan = { name: string; targetRows: number; artifactRows: number; exists: boolean };
export type CategoryPlan = { name: RestoreCategory; selected: boolean; tables: TablePlan[] };

export type RestorePlan = {
  manifest: ExportManifest;
  mode: RestoreMode;
  categories: CategoryPlan[];
  restoresKeys: boolean;
  restoresSecret: boolean;
  restoresFiles: boolean;
  sameKey: boolean;
  plugins: { slug: string; version: string; enabled: boolean }[];
  refusals: string[];
  warnings: string[];
};

export type PluginOutcome = { slug: string; version: string; action: string; error?: string };

export type RestoreState = 'running' | 'restarting' | 'reconciling' | 'done' | 'failed';

export type RestoreStatus = {
  id: string;
  state: RestoreState;
  actor: string;
  mode: RestoreMode;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  refusals?: string[];
  result?: { rows: number; tables: number; anchorId?: string };
  plugins?: PluginOutcome[];
  progress?: { phase: string; item?: string; done: number; total: number };
};

const ACTIVE: RestoreState[] = ['running', 'restarting', 'reconciling'];

export function useRestoreStatus() {
  return useQuery({
    queryKey: ['instance-backups', 'restore-status'],
    queryFn: () => api.get<RestoreStatus | null>('/instance-backups/restore/status').then((r) => r.data ?? null),
    refetchInterval: (query) => (ACTIVE.includes(query.state.data?.state as RestoreState) ? 2_000 : false),
    retry: true,
  });
}

export function useUploadExport() {
  return useMutation({
    mutationFn: (file: Blob) =>
      api.putBinary<{ uploadId: string; manifest: ExportManifest }>('/instance-backups/uploads', file).then(refusalOf),
  });
}

export function usePlanRestore() {
  return useMutation({
    mutationFn: (input: RestoreInput) => api.post<RestorePlan>('/instance-backups/restore/plan', input).then(refusalOf),
  });
}

export function useStartRestore() {
  return useMutation({
    mutationFn: (input: RestoreInput) => api.post<RestoreStatus>('/instance-backups/restore', input).then(refusalOf),
  });
}

export function isActive(currentStatus: RestoreStatus | null | undefined): boolean {
  return !!currentStatus && ACTIVE.includes(currentStatus.state);
}
