// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { KEYS, invalidateVM } from './vmQueryKeys';

/** Mirrors `MigrateRequest` in the plugin's backend `migrate.go`. */
export interface MigrateInput {
  targetEnvironmentId: string;
  targetPoolId: string;
  targetPoolBackend: string;
  targetPoolTarget: string;
  live: boolean;
  targetAddress?: string;
  maxBandwidthMb?: number;
  maxDowntimeMs?: number;
  nics?: MigrateNIC[];
}

export interface MigrateNIC {
  nicId: string;
  networkId: string;
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
}

/** Mirrors `MigrationStatus` in `migrate.go`; it never carries a token or key. */
export interface MigrationStatus {
  id: string;
  vmId: string;
  sourceEnvironmentId: string;
  targetEnvironmentId: string;
  mode: 'cold' | 'live';
  state: 'running' | 'completed' | 'failed' | 'cancelled';
  step?: string;
  percent: number;
  error?: string;
  warning?: string;
  startedAt: string;
  finishedAt?: string;
}

/** A host as the compute foundation lists it, for the migration target picker. */
export interface ComputeHost {
  environmentId: string;
  name?: string;
  connectionType?: string;
  computeEnabled: boolean;
  available: boolean;
}

const migrationKey = (id: string) => [...KEYS.detail(id), 'migration'] as const;

/** The running or last migration of a guest; polled while one is running. */
export function useMigrationStatus(id: string, enabled: boolean) {
  return useQuery({
    queryKey: migrationKey(id),
    enabled: enabled && Boolean(id),
    queryFn: async () => assertSuccess(await api.get<MigrationStatus | null>(`/vms/${id}/migration`)),
    refetchInterval: (query) => (query.state.data?.state === 'running' ? 1500 : false),
  });
}

/**
 * Hosts a guest can move to: compute turned on and the agent reachable now.
 * `available` is false for anything but a connected agent host, so it is the
 * agent test as well; `connectionType` is only joined for display.
 */
export function useMigrationTargets(sourceEnvironmentId: string, enabled: boolean) {
  return useQuery({
    queryKey: ['compute', 'environments'],
    enabled,
    queryFn: async () => assertSuccess(await api.get<ComputeHost[]>('/compute/environments')) ?? [],
    select: (hosts: ComputeHost[]) =>
      hosts.filter((host) => host.environmentId !== sourceEnvironmentId && host.computeEnabled && host.available),
  });
}

export function useMigrateVM() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('vm');
  return useMutation({
    meta: { success: t('msg.vm.migration_started') },
    mutationFn: async (input: { id: string; body: MigrateInput }) =>
      assertSuccess(await api.post<MigrationStatus>(`/vms/${input.id}/migrate`, input.body)),
    onSuccess: (started, input) => {
      queryClient.setQueryData(migrationKey(input.id), started);
      invalidateVM(queryClient, input.id);
    },
  });
}

export function useCancelMigration() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('vm');
  return useMutation({
    meta: { success: t('msg.vm.migration_cancel_requested') },
    mutationFn: async (id: string) => assertSuccess(await api.post<null>(`/vms/${id}/migration/cancel`, {})),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: migrationKey(id) });
    },
  });
}
