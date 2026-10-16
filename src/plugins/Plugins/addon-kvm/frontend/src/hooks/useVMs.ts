// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { CreateVMInput, Disk, MetricsPoint, NIC, Snapshot, Stats, VM } from '../types';
import { KEYS, params, invalidateVM } from './vmQueryKeys';

/**
 * Server state for virtual machines.
 *
 * The list polls, because guest state changes without anyone asking — a guest
 * crashes, a host reboots, an autostart fires. Everything else is fetched on
 * demand and invalidated by the mutation that changed it.
 */

/** Every guest, or every template. */
export function useVMs(envId?: string, templates = false) {
  return useQuery({
    queryKey: KEYS.list(envId, templates),
    queryFn: async () =>
      assertSuccess(await api.get<VM[]>('/vms', params({ env: envId, templates: templates ? 'true' : undefined }))) ??
      [],
    // Guest state moves on its own, so the list keeps up without the operator
    // reaching for refresh.
    refetchInterval: 10_000,
  });
}

/** One guest with its hardware, snapshots and first-boot settings. */
export function useVM(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.detail(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<VM>(`/vms/${id}`)),
    refetchInterval: 10_000,
  });
}

/**
 * Live resource usage.
 *
 * Enabled only while the guest is actually running: sampling a stopped guest
 * costs a round trip to the host and returns nothing useful.
 */
export function useVMStats(id: string | undefined, running: boolean) {
  return useQuery({
    queryKey: KEYS.stats(id ?? ''),
    enabled: Boolean(id) && running,
    queryFn: async () => assertSuccess(await api.get<Stats>(`/vms/${id}/stats`)),
    refetchInterval: 5_000,
    // A failed sample is not worth retrying hard — the next interval will try
    // again anyway.
    retry: 0,
  });
}

/**
 * The stored last hour, so the charts open with history. Refreshed on the
 * sampler's own 30s cadence; the live 5s polls fill in between.
 */
export function useVMStatsHistory(id: string | undefined, running: boolean) {
  return useQuery({
    queryKey: KEYS.statsHistory(id ?? ''),
    enabled: Boolean(id) && running,
    queryFn: async () => assertSuccess(await api.get<MetricsPoint[]>(`/vms/${id}/stats/history`)) ?? [],
    refetchInterval: 30_000,
    retry: 0,
  });
}

export function useVMDisks(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.disks(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<Disk[]>(`/vms/${id}/disks`)) ?? [],
  });
}

export function useVMNICs(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.nics(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<NIC[]>(`/vms/${id}/nics`)) ?? [],
  });
}

export function useVMSnapshots(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.snapshots(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<Snapshot[]>(`/vms/${id}/snapshots`)) ?? [],
  });
}

export function useCreateVM() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateVMInput) => assertSuccess(await api.post<VM>('/vms', body)),
    onSuccess: () => invalidateVM(queryClient),
  });
}

export function useUpdateVM() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('vm');
  return useMutation({
    meta: { success: t('settings.saved') },
    mutationFn: async (input: { id: string; body: Record<string, unknown> }) =>
      assertSuccess(await api.put<VM>(`/vms/${input.id}`, input.body)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

/** Start, shutdown, stop, reset, pause, resume. */
export type LifecycleAction = 'start' | 'shutdown' | 'stop' | 'reset' | 'pause' | 'resume';

export function useVMLifecycle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; action: LifecycleAction; graceSeconds?: number }) =>
      assertSuccess(
        await api.post<null>(
          `/vms/${input.id}/${input.action}`,
          input.action === 'shutdown' ? { graceSeconds: input.graceSeconds } : {},
        ),
      ),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useDeleteVM() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; deleteVolumes?: boolean }) =>
      assertSuccess(
        await api.del<null>(`/vms/${input.id}`, input.deleteVolumes ? { deleteVolumes: 'true' } : undefined),
      ),
    onSuccess: () => invalidateVM(queryClient),
  });
}

export * from './useVMHardware';
export * from './useVMSnapshots';
export * from './useVMDevices';
