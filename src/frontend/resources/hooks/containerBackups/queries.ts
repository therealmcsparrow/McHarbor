// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQueries, useQuery } from '@tanstack/react-query';

import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerBackupOptions, ContainerBackupPlan, ContainerBackupRun } from './types';

export function useContainerBackupOptions(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-options', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupOptions>(`/containers/${containerId}/backups/options`, envId ? { env: envId } : {})
        .then(assertSuccess),
    enabled: !!containerId,
  });
}

export function useContainerBackupPlans(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-plans', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupPlan[]>('/container-backups', envId ? { env: envId, containerId } : { containerId })
        .then((r) => assertSuccess(r) ?? []),
    enabled: !!containerId,
  });
}

// useAllContainerBackupPlans returns backup plans across all
// environments (or one environment if `envId` is provided). When
// `envId === ''` (the global "All environments" selection in the
// header) the hook fans out one request per environment in parallel
// via useQueries and aggregates the results; with a single envId it
// falls back to one request, matching the existing single-env hook.
export function useAllContainerBackupPlans(envId: string | undefined | null) {
  const environments = useEnvironmentStore((s) => s.environments);
  const envs = envId && envId.length > 0 ? environments.filter((env) => env.id === envId) : environments;
  const queries = useQueries({
    queries: envs.map((env) => ({
      // Nest under the same root key as the per-container
      // `container-backup-plans` query so the run/plan mutations'
      // broad `invalidateQueries(['container-backup-plans'])`
      // invalidations match every env's row.
      queryKey: ['container-backup-plans', 'all', env.id],
      queryFn: () =>
        api
          .get<ContainerBackupPlan[]>(`/container-backups?env=${encodeURIComponent(env.id)}`)
          .then((r) => assertSuccess(r) ?? []),
      staleTime: 15_000,
    })),
    combine: (results) => {
      const plans: ContainerBackupPlan[] = [];
      for (const r of results) {
        if (r.data) plans.push(...r.data);
      }
      plans.sort((a, b) => a.name.localeCompare(b.name));
      const isLoading = results.some((r) => r.isLoading);
      return { data: plans, isLoading };
    },
  });
  return queries as { data: ContainerBackupPlan[]; isLoading: boolean };
}

export function useContainerBackupRuns(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-runs', envId, containerId],
    queryFn: () =>
      api
        .get<ContainerBackupRun[]>('/container-backups/runs', envId ? { env: envId, containerId } : { containerId })
        .then(assertSuccess),
    enabled: !!containerId,
    // Poll while a backup is running OR while any destination is
    // uploading. The latter covers destination retry uploads that
    // run against a previously-completed run (so run.status is
    // 'success' / 'cancelled' / 'failure') — without this branch
    // the operator never sees byte progress for a retry because
    // the run status alone doesn't change.
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      const hasRunningRun = data.some((run) => run.status === 'running');
      const hasUploadingDestination = data.some((run) =>
        (run.destinations ?? []).some((destination) => destination.status === 'uploading'),
      );
      return hasRunningRun || hasUploadingDestination ? 2000 : false;
    },
  });
}

// useAllContainerBackupRuns returns recent runs across all
// environments (or one if `envId` is set). Like its plans sibling,
// polls while any run is uploading so live progress is visible.
export function useAllContainerBackupRuns(envId: string | undefined | null) {
  const environments = useEnvironmentStore((s) => s.environments);
  const envs = envId && envId.length > 0 ? environments.filter((env) => env.id === envId) : environments;
  const queries = useQueries({
    queries: envs.map((env) => ({
      // See the comment in useAllContainerBackupPlans — nest
      // under the same root key so broad invalidations hit this
      // query too (otherwise the Backups page table never
      // refetches after a delete / cancel / etc.).
      queryKey: ['container-backup-runs', 'all', env.id],
      queryFn: () =>
        api
          .get<ContainerBackupRun[]>(`/container-backups/runs?env=${encodeURIComponent(env.id)}`)
          .then((r) => assertSuccess(r) ?? []),
      staleTime: 10_000,
      refetchInterval: (query: { state: { data?: ContainerBackupRun[] | undefined } }) => {
        const data = query.state.data;
        if (!data) return false;
        const hasRunning = data.some((run) => run.status === 'running');
        const hasUploading = data.some((run) =>
          (run.destinations ?? []).some((destination) => destination.status === 'uploading'),
        );
        return hasRunning || hasUploading ? 2000 : false;
      },
    })),
    combine: (results) => {
      const runs: ContainerBackupRun[] = [];
      for (const r of results) {
        if (r.data) runs.push(...r.data);
      }
      runs.sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime());
      const isLoading = results.some((r) => r.isLoading);
      return { data: runs, isLoading };
    },
  });
  return queries as { data: ContainerBackupRun[]; isLoading: boolean };
}

export function useContainerBackupRun(containerId: string, runId?: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-backup-run', envId, containerId, runId],
    queryFn: () =>
      api
        .get<ContainerBackupRun>(
          `/container-backups/runs/${encodeURIComponent(runId ?? '')}`,
          envId ? { env: envId } : {},
        )
        .then(assertSuccess),
    enabled: !!containerId && !!runId,
    // Keep polling until a terminal status arrives. Checking only for
    // `status === "running"` meant a query that had not resolved yet — or
    // one whose first fetch failed — never started polling at all, which
    // left the restore dialog frozen on its empty state with no progress
    // and no completion.
    refetchInterval: (query) => {
      // A run that has genuinely gone (deleted, or never existed) settles
      // into the error state after the retries; polling it forever would
      // just hammer the API.
      if (query.state.status === 'error') return false;
      const currentStatus = query.state.data?.status;
      // 2s, not 1s: progress does not need sub-second resolution, and this
      // endpoint shares one SQLite file with the running backup/restore that
      // is trying to write its own progress and terminal state.
      if (!currentStatus) return 2000;
      return currentStatus === 'running' ? 2000 : false;
    },
  });
}
