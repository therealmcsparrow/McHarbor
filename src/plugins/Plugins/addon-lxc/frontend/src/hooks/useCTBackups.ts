// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type BackupKind = 'full' | 'incremental';
export type BackupStatus = 'running' | 'success' | 'failure' | 'cancelled';
export type BackupConsistency = 'snapshot' | 'frozen' | 'filesystem' | 'live-copy';
export type DestinationStatus = 'uploading' | 'success' | 'failure';
export type BackupPart = 'manifest' | 'rootfs';

export interface BackupPlan {
  id: string;
  containerId: string;
  name: string;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  fullEvery: number;
  destinationIds: string[];
  lastRunAt?: string;
  nextRunAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BackupRunDestination {
  id: string;
  runId: string;
  locationId: string;
  locationName?: string;
  part: BackupPart;
  status: DestinationStatus;
  remotePath?: string;
  keyId?: string;
  bytesUploaded: number;
  error?: string;
  uploadedAt?: string;
}

export interface BackupRun {
  id: string;
  planId?: string;
  containerId: string;
  containerName?: string;
  environmentId: string;
  operation: 'backup' | 'restore';
  sourceRunId?: string;
  status: BackupStatus;
  kind: BackupKind;
  chainId?: string;
  fellBackToFull?: boolean;
  fallbackReason?: string;
  consistency: BackupConsistency;
  bytesTotal: number;
  targetContainerId?: string;
  idShift?: 'none' | 'shifted' | 'skipped';
  error?: string;
  errorHint?: 'backup_key_missing';
  progressStage?: string;
  progressMessage?: string;
  progressUpdatedAt?: string;
  destinations: BackupRunDestination[];
  startedAt: string;
  completedAt?: string;
  durationMs: number;
}

export interface BackupPlanInput {
  name: string;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  fullEvery: number;
  destinationIds: string[];
}

const KEYS = {
  plans: (containerId: string) => ['containers', 'backup-plans', containerId] as const,
  runs: (containerId: string) => ['containers', 'backup-runs', containerId] as const,
};

export function useBackupPlans(containerId: string) {
  return useQuery({
    queryKey: KEYS.plans(containerId),
    queryFn: async () =>
      assertSuccess(await api.get<BackupPlan[]>(`/system-containers/${containerId}/backup-plans`)) ?? [],
  });
}

export function useBackupRuns(containerId: string) {
  return useQuery({
    queryKey: KEYS.runs(containerId),
    queryFn: async () => assertSuccess(await api.get<BackupRun[]>(`/system-containers/${containerId}/backups`)) ?? [],
    refetchInterval: (query) => ((query.state.data ?? []).some((run) => run.status === 'running') ? 5_000 : false),
  });
}

function invalidate(queryClient: ReturnType<typeof useQueryClient>, containerId: string) {
  void queryClient.invalidateQueries({ queryKey: KEYS.plans(containerId) });
  void queryClient.invalidateQueries({ queryKey: KEYS.runs(containerId) });
}

export function useCreateBackupPlan(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: BackupPlanInput) =>
      assertSuccess(await api.post<BackupPlan>(`/system-containers/${containerId}/backup-plans`, input)),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}

export function useUpdateBackupPlan(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ planId, input }: { planId: string; input: Partial<BackupPlanInput> }) =>
      assertSuccess(await api.put<BackupPlan>(`/system-containers/backup-plans/${planId}`, input)),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}

export function useDeleteBackupPlan(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (planId: string) => assertSuccess(await api.del(`/system-containers/backup-plans/${planId}`)),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}

export function useRunBackup(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ planId, kind }: { planId?: string; kind?: BackupKind }) =>
      assertSuccess(
        await api.post<BackupRun>(
          `/system-containers/${containerId}/backups${planId ? `?plan=${planId}` : ''}`,
          kind ? { kind } : {},
        ),
      ),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}

export function useDeleteBackupRun(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (runId: string) => assertSuccess(await api.del(`/system-containers/backups/${runId}`)),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}

export function useRestoreBackup(containerId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      runId,
      confirmName,
      targetContainerId,
    }: {
      runId: string;
      confirmName: string;
      targetContainerId?: string;
    }) =>
      assertSuccess(
        await api.post<BackupRun>(`/system-containers/backups/${runId}/restore`, {
          confirmName,
          targetContainerId,
        }),
      ),
    onSuccess: () => invalidate(queryClient, containerId),
  });
}
