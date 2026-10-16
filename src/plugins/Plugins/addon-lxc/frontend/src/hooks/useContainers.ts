// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { invalidate, KEYS, params } from './containerQueryShared';

import type {
  Container,
  ConsoleKind,
  ConsoleTicket,
  CreateContainerInput,
  Limits,
  LimitsResponse,
  Process,
  Stats,
} from '../types';

export * from './useContainerChildren';

/**
 * Server state for system containers.
 *
 * The list polls, because container state changes without anyone asking — a
 * container's init exits, a host reboots, an autostart fires. Everything else
 * is fetched on demand and invalidated by the mutation that changed it.
 */

/** Every container, or every template. */
export function useContainers(envId?: string, templates = false) {
  return useQuery({
    queryKey: KEYS.list(envId, templates),
    queryFn: async () =>
      assertSuccess(
        await api.get<Container[]>(
          '/system-containers',
          params({ env: envId, templates: templates ? 'true' : undefined }),
        ),
      ) ?? [],
    // Container state moves on its own, so the list keeps up without the
    // operator reaching for refresh.
    refetchInterval: 10_000,
  });
}

/** One container with its interfaces, mounts and snapshots. */
export function useContainer(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.detail(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<Container>(`/system-containers/${id}`)),
    refetchInterval: 10_000,
  });
}

/**
 * Live resource usage.
 *
 * Enabled only while the container is actually executing: sampling a stopped
 * container costs a round trip to the host and returns nothing useful. A frozen
 * container still counts — it holds its memory and its pids, and that is
 * exactly what somebody looking at this panel wants to see.
 */
export function useContainerStats(id: string | undefined, live: boolean) {
  return useQuery({
    queryKey: KEYS.stats(id ?? ''),
    enabled: Boolean(id) && live,
    queryFn: async () => assertSuccess(await api.get<Stats>(`/system-containers/${id}/stats`)),
    refetchInterval: 5_000,
    // A failed sample is not worth retrying hard — the next interval will try
    // again anyway.
    retry: 0,
  });
}

/**
 * What is running inside a container.
 *
 * Polled more slowly than stats: a process list is a bigger answer and an
 * operator reads it rather than watching it move.
 */
export function useContainerProcesses(id: string | undefined, live: boolean) {
  return useQuery({
    queryKey: KEYS.processes(id ?? ''),
    enabled: Boolean(id) && live,
    queryFn: async () => assertSuccess(await api.get<Process[]>(`/system-containers/${id}/processes`)) ?? [],
    refetchInterval: 15_000,
    retry: 0,
  });
}

export function useCreateContainer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateContainerInput) =>
      assertSuccess(await api.post<Container>('/system-containers', body)),
    onSuccess: () => invalidate(queryClient),
  });
}

export function useUpdateContainer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; body: Record<string, unknown> }) =>
      assertSuccess(await api.put<Container>(`/system-containers/${input.id}`, input.body)),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export type LifecycleAction = 'start' | 'shutdown' | 'stop' | 'restart' | 'freeze' | 'unfreeze';

export function useLifecycle() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; action: LifecycleAction; graceSeconds?: number }) =>
      assertSuccess(
        await api.post<Container>(
          `/system-containers/${input.id}/${input.action}`,
          input.action === 'shutdown' ? { graceSeconds: input.graceSeconds } : {},
        ),
      ),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

/**
 * Change a container's cgroup limits.
 *
 * The response carries what the kernel actually applied and the names of any
 * field it clamped, which is why this returns the body rather than discarding
 * it: the panel shows the applied values, not the requested ones.
 */
export function useUpdateLimits() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; limits: Limits }) =>
      assertSuccess(await api.put<LimitsResponse>(`/system-containers/${input.id}/limits`, input.limits)),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useDeleteContainer() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; keepRootfs?: boolean }) =>
      assertSuccess(
        await api.del<null>(`/system-containers/${input.id}`, input.keepRootfs ? { keepRootfs: 'true' } : undefined),
      ),
    onSuccess: () => invalidate(queryClient),
  });
}

export function useClone() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; name: string; hostname?: string; start?: boolean; linked?: boolean }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Container>(`/system-containers/${id}/clone`, body));
    },
    onSuccess: () => invalidate(queryClient),
  });
}

export function useSetTemplate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; isTemplate: boolean }) =>
      assertSuccess(await api.post<null>(`/system-containers/${input.id}/template`, { isTemplate: input.isTemplate })),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

/**
 * Ask the AI router what is wrong with a container.
 *
 * A mutation rather than a query on purpose: it costs tokens, so it runs when
 * the operator asks and never on render.
 */
export function useDiagnose() {
  return useMutation({
    mutationFn: async (id: string) =>
      assertSuccess(
        await api.post<{ available: boolean; diagnosis?: string }>(`/system-containers/${id}/diagnose`, {}),
      ),
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
    mutationFn: async (input: { environmentId: string; containerId: string; kind: ConsoleKind }) =>
      assertSuccess(
        await api.post<ConsoleTicket>('/compute/console/ticket', {
          envId: input.environmentId,
          guestKind: 'ct',
          guestId: input.containerId,
          kind: input.kind,
        }),
      ),
  });
}
