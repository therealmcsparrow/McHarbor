// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface SnapshotConfig {
  id: string;
  environmentId: string;
  name: string;
  driver: string;
  pluginSlug: string;
  enabled: boolean;
  poolName?: string;
  datasetPrefix?: string;
  sshTarget?: string;
  useLxd: boolean;
  config: Record<string, string>;
  /** Secret fields with a stored value; the values themselves are never returned. */
  secretsSet?: string[];
  lastProbedAt?: string;
  lastProbe?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SnapshotConfigWrite {
  name: string;
  driver: string;
  pluginSlug: string;
  enabled?: boolean;
  poolName?: string;
  datasetPrefix?: string;
  sshTarget?: string;
  useLxd?: boolean;
  config?: Record<string, string>;
}

export interface SnapshotProbeResult {
  driver: string;
  available: boolean;
  toolPath?: string;
  version?: string;
  targets: Array<{ name: string; mountpoint?: string; usedBytes?: number; driver: string }>;
  error?: string;
}

export interface RecentSnapshot {
  runId: string;
  planId: string;
  planName?: string;
  containerName?: string;
  environmentId: string;
  snapshotName: string;
  snapshotDriver: string;
  snapshotMountPath?: string;
  snapshotCreatedAt: string;
  snapshotRemovedAt?: string;
  status: 'success' | 'failure' | 'running';
  startedAt: string;
  completedAt?: string;
}

function listKey(envId: string | null) {
  return ['snapshotConfigs', envId ?? ''] as const;
}

function recentKey(envId: string | null, limit: number) {
  return ['snapshotRecent', envId ?? '', limit] as const;
}

export function useSnapshotConfigs(envId: string | null) {
  return useQuery({
    queryKey: listKey(envId),
    enabled: !!envId,
    queryFn: async () => {
      const res = await api.get<SnapshotConfig[]>(`/container-backups/snapshot-configs?env=${envId}`);
      return assertSuccess(res);
    },
  });
}

export function useCreateSnapshotConfig(envId: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body: SnapshotConfigWrite) => {
      const res = await api.post<SnapshotConfig>(`/container-backups/snapshot-configs?env=${envId}`, body);
      return assertSuccess(res);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: listKey(envId) });
    },
  });
}

export function useUpdateSnapshotConfig(envId: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id: string; body: SnapshotConfigWrite }) => {
      const res = await api.put<SnapshotConfig>(`/container-backups/snapshot-configs/${id}?env=${envId}`, body);
      return assertSuccess(res);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: listKey(envId) });
    },
  });
}

export function useDeleteSnapshotConfig(envId: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const res = await api.del(`/container-backups/snapshot-configs/${id}?env=${envId}`);
      return assertSuccess(res);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: listKey(envId) });
    },
  });
}

export function useProbeSnapshotConfig(envId: string | null) {
  return useMutation({
    mutationFn: async (id: string) => {
      const res = await api.post<SnapshotProbeResult>(`/container-backups/snapshot-configs/${id}/probe?env=${envId}`);
      return assertSuccess(res);
    },
  });
}

export function useRecentSnapshots(envId: string | null, limit = 20) {
  return useQuery({
    queryKey: recentKey(envId, limit),
    enabled: !!envId,
    queryFn: async () => {
      const res = await api.get<RecentSnapshot[]>(`/container-backups/snapshots/recent?env=${envId}&limit=${limit}`);
      return assertSuccess(res);
    },
  });
}

/**
 * One snapshot driver's availability on one environment, as reported by
 * the host or, for an agent environment, by the agent itself. A driver
 * that is missing is still listed: an operator who cannot see that
 * btrfs exists has no way to learn that installing it would help.
 */
export interface SnapshotDriverStatus {
  driver: string;
  displayName: string;
  available: boolean;
  origin: 'agent' | 'local' | 'plugin';
  toolPath?: string;
  version?: string;
  error?: string;
  pauseFree: boolean;
  targets?: Array<{ name: string; mountpoint?: string; usedBytes?: number; driver: string }>;
}

function driversKey(envId: string | null) {
  return ['snapshotDrivers', envId ?? ''] as const;
}

export function useSnapshotDrivers(envId: string | null) {
  return useQuery({
    queryKey: driversKey(envId),
    enabled: !!envId,
    // Detection shells out to the host or crosses the agent link, so it
    // is refetched on demand rather than on every focus change.
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const res = await api.get<SnapshotDriverStatus[]>(`/container-backups/snapshot-drivers?env=${envId}`);
      return assertSuccess(res);
    },
  });
}
