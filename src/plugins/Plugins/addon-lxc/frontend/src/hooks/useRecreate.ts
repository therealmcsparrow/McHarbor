// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { BackupRun } from './useCTBackups';
import type { Container, CreateContainerInput } from '../types';

export type RecreateWarning =
  | 'name_invalid'
  | 'init_unknown'
  | 'pool_target_unknown'
  | 'nic_dropped'
  | 'mount_dropped'
  | 'limits_adjusted';

export interface RecreateDraft {
  sourceRunId: string;
  sourceContainerId: string;
  takenAt?: string;
  osFamily?: string;
  osVersion?: string;
  create: CreateContainerInput;
  warnings: RecreateWarning[];
}

export interface RecreateBody {
  create: CreateContainerInput;
  osFamily?: string;
  osVersion?: string;
  startAfterRestore?: boolean;
}

export interface RecreateResult {
  container: Container;
  run: BackupRun;
}

export function useOrphanedBackups(environmentId?: string, enabled = true) {
  return useQuery({
    queryKey: ['containers', 'orphaned-backups', environmentId ?? 'all'],
    enabled,
    queryFn: async () =>
      assertSuccess(
        await api.get<BackupRun[]>(
          '/system-containers/orphaned-backups',
          environmentId ? { env: environmentId } : undefined,
        ),
      ) ?? [],
  });
}

export function useBackupManifest(runId: string) {
  return useQuery({
    queryKey: ['containers', 'backup-manifest', runId],
    retry: false,
    staleTime: 60_000,
    queryFn: async () => assertSuccess(await api.get<RecreateDraft>(`/system-containers/backups/${runId}/manifest`)),
  });
}

export function useRecreateFromBackup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ runId, body }: { runId: string; body: RecreateBody }) =>
      assertSuccess(await api.post<RecreateResult>(`/system-containers/backups/${runId}/recreate`, body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });
}
