// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type BackupKind = 'full' | 'incremental';
export type BackupStatus = 'running' | 'success' | 'failure' | 'cancelled';
export type DestinationStatus = 'uploading' | 'success' | 'failure';

export interface BackupPlan {
  id: string;
  vmId: string;
  name: string;
  cron?: string;
  enabled: boolean;
  retentionCount: number;
  retentionDays: number;
  fullEvery: number;
  quiesce: boolean;
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
  diskId?: string;
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
  vmId: string;
  operation: 'backup' | 'restore';
  sourceRunId?: string;
  status: BackupStatus;
  kind: BackupKind;
  chainId?: string;
  /** True when an incremental was asked for and a full copy was taken. */
  fellBackToFull?: boolean;
  fallbackReason?: string;
  consistency: string;
  bytesTotal: number;
  includedDisks?: string[];
  error?: string;
  progressStage?: string;
  progressMessage?: string;
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
  quiesce: boolean;
  destinationIds: string[];
}

export function useBackupPlans(vmId: string) {
  return useQuery({
    queryKey: ['kvm', 'backup-plans', vmId],
    queryFn: async () => assertSuccess(await api.get<BackupPlan[]>(`/vms/${vmId}/backup-plans`)),
  });
}

/**
 * A machine's backup history.
 *
 * Polled while anything is running: a full copy of a large guest takes hours,
 * and the run row is the only place its progress appears.
 */
export function useBackupRuns(vmId: string) {
  return useQuery({
    queryKey: ['kvm', 'backup-runs', vmId],
    queryFn: async () => assertSuccess(await api.get<BackupRun[]>(`/vms/${vmId}/backups`)),
    refetchInterval: (query) => ((query.state.data ?? []).some((run) => run.status === 'running') ? 5_000 : false),
  });
}

function invalidate(queryClient: ReturnType<typeof useQueryClient>, vmId: string) {
  void queryClient.invalidateQueries({
    queryKey: ['kvm', 'backup-plans', vmId],
  });
  void queryClient.invalidateQueries({
    queryKey: ['kvm', 'backup-runs', vmId],
  });
}

export function useCreateBackupPlan(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: BackupPlanInput) =>
      assertSuccess(await api.post<BackupPlan>(`/vms/${vmId}/backup-plans`, input)),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}

export function useUpdateBackupPlan(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ planId, input }: { planId: string; input: Partial<BackupPlanInput> }) =>
      assertSuccess(await api.put<BackupPlan>(`/vms/backup-plans/${planId}`, input)),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}

export function useDeleteBackupPlan(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (planId: string) => assertSuccess(await api.del(`/vms/backup-plans/${planId}`)),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}

export function useRunBackup(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ planId, kind }: { planId?: string; kind?: BackupKind }) =>
      assertSuccess(
        await api.post<BackupRun>(`/vms/${vmId}/backups${planId ? `?plan=${planId}` : ''}`, kind ? { kind } : {}),
      ),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}

export function useDeleteBackupRun(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (runId: string) => assertSuccess(await api.del(`/vms/backups/${runId}`)),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}

export function useRestoreBackup(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ runId, diskIds }: { runId: string; diskIds?: string[] }) =>
      assertSuccess(
        await api.post<BackupRun>(`/vms/backups/${runId}/restore`, {
          diskIds,
          confirm: true,
        }),
      ),
    onSuccess: () => invalidate(queryClient, vmId),
  });
}
