// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type BackupLogSeverity = 'info' | 'success' | 'warning' | 'error';

export type BackupLog = {
  id: string;
  environmentId?: string;
  planId?: string;
  planName?: string;
  runId?: string;
  containerId?: string;
  containerName?: string;
  action: string;
  phase: string;
  severity: BackupLogSeverity;
  message: string;
  source: 'backup' | 'audit';
  createdAt: string;
};

export type BackupLogListResponse = {
  items: BackupLog[];
  total: number;
  page: number;
  perPage: number;
};

export type BackupLogFilters = {
  envId?: string;
  planId?: string;
  runId?: string;
  severity?: BackupLogSeverity | '';
  action?: string;
  search?: string;
  from?: string;
  until?: string;
  page?: number;
  perPage?: number;
};

export function useBackupLogs(filters: BackupLogFilters = {}) {
  const search = new URLSearchParams();
  if (filters.envId) search.set('envId', filters.envId);
  if (filters.planId) search.set('planId', filters.planId);
  if (filters.runId) search.set('runId', filters.runId);
  if (filters.severity) search.set('severity', filters.severity);
  if (filters.action) search.set('action', filters.action);
  if (filters.search) search.set('search', filters.search);
  if (filters.from) search.set('from', filters.from);
  if (filters.until) search.set('until', filters.until);
  if (filters.page) search.set('page', String(filters.page));
  if (filters.perPage) search.set('perPage', String(filters.perPage));
  const qs = search.toString();
  return useQuery({
    queryKey: ['backup-logs', filters],
    queryFn: () =>
      api
        .get<BackupLogListResponse>(`/backup-logs${qs ? `?${qs}` : ''}`)
        .then((r) => r.data),
    refetchInterval: 5_000,
  });
}
