// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { refusalOf, type KeyKind } from './useInstanceBackups';

export type ExportSchedule = {
  id: string;
  name: string;
  cron: string;
  enabled: boolean;
  keyKind: KeyKind;
  keyId: string;
  includeMetrics: boolean;
  locationIds?: string[];
  retentionCount: number;
  retentionDays: number;
  verifyAfter: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  lastStatus: string;
  lastError: string;
  createdBy: string;
  createdAt: string;
};

export type ScheduleInput = {
  name: string;
  cron: string;
  enabled: boolean;
  passphrase?: string;
  keyFile?: string;
  includeMetrics: boolean;
  locationIds?: string[];
  retentionCount: number;
  retentionDays: number;
  verifyAfter: boolean;
};

const KEY = ['instance-backups', 'schedules'] as const;

export function useExportSchedules(enabled = true) {
  return useQuery({
    queryKey: KEY,
    enabled,
    queryFn: () => api.get<ExportSchedule[]>('/instance-backups/schedules').then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useSaveSchedule() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id?: string; input: ScheduleInput }) =>
      (id
        ? api.put<ExportSchedule>(`/instance-backups/schedules/${encodeURIComponent(id)}`, input)
        : api.post<ExportSchedule>('/instance-backups/schedules', input)
      ).then(refusalOf),
    onSuccess: () => client.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteSchedule() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/instance-backups/schedules/${encodeURIComponent(id)}`).then(assertSuccess),
    onSuccess: () => client.invalidateQueries({ queryKey: KEY }),
  });
}

export function useRunSchedule() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<ExportSchedule>(`/instance-backups/schedules/${encodeURIComponent(id)}/run`).then(assertSuccess),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: KEY });
      void client.invalidateQueries({ queryKey: ['instance-backups'] });
    },
  });
}
