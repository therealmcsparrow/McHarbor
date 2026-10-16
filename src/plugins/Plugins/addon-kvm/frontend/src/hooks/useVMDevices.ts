// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ConsoleKind, ConsoleTicket, HostDevice, VM } from '../types';
import { KEYS, invalidateVM } from './vmQueryKeys';

export function useHostDevices(envId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['vm', 'host-devices', envId ?? ''],
    enabled: Boolean(envId) && enabled,
    queryFn: async () =>
      assertSuccess(await api.get<HostDevice[]>(`/vms/host-devices?env=${encodeURIComponent(envId ?? '')}`)) ?? [],
    staleTime: 30_000,
  });
}

export function useAttachDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; kind: string; hostAddress: string; description?: string }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post(`/vms/${id}/devices`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useDetachDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; deviceId: string }) =>
      assertSuccess(await api.del<null>(`/vms/${input.id}/devices/${input.deviceId}`)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

/**
 * Ask the AI router what is wrong with a guest.
 *
 * A mutation rather than a query on purpose: it costs tokens, so it runs when
 * the operator asks and never on render.
 */
export function useDiagnose() {
  return useMutation({
    mutationFn: async (id: string) =>
      assertSuccess(await api.post<{ available: boolean; diagnosis?: string }>(`/vms/${id}/diagnose`, {})),
  });
}

/**
 * Request a console ticket.
 *
 * The ticket is single-use and lives 30 seconds, so this runs immediately
 * before the WebSocket opens rather than being cached. It is the host's
 * gateway, not this plugin, that issues it — which is why the path has no
 * plugin prefix.
 */
export function useConsoleTicket() {
  return useMutation({
    mutationFn: async (input: { environmentId: string; guestId: string; kind: ConsoleKind; channels?: number }) =>
      assertSuccess(
        await api.post<ConsoleTicket>('/compute/console/ticket', {
          envId: input.environmentId,
          guestKind: 'vm',
          guestId: input.guestId,
          kind: input.kind,
          channels: input.channels,
        }),
      ),
  });
}

/**
 * A guest polled once a second, for as long as something is happening to it
 * that it reports progress for.
 *
 * `useVM` refetches every ten seconds, which is right for a detail page and
 * useless for a bar that is supposed to move. This is the same row, asked for
 * more often, and only while it is worth asking.
 */
export function useVMProgress(id: string, active: boolean) {
  return useQuery({
    queryKey: [...KEYS.detail(id), 'progress'],
    enabled: active && Boolean(id),
    refetchInterval: active ? 1000 : false,
    queryFn: async () => assertSuccess(await api.get<VM>(`/vms/${id}`)),
  });
}
