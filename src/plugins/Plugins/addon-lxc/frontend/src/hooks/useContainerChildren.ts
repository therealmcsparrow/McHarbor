// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { CreateMountInput, CreateNICInput, Mount, NIC, Snapshot } from '../types';
import { invalidate, KEYS } from './containerQueryShared';

export function useContainerNICs(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.nics(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<NIC[]>(`/system-containers/${id}/nics`)) ?? [],
  });
}

export function useContainerMounts(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.mounts(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<Mount[]>(`/system-containers/${id}/mounts`)) ?? [],
  });
}

export function useContainerSnapshots(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.snapshots(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<Snapshot[]>(`/system-containers/${id}/snapshots`)) ?? [],
  });
}

export function useAddNIC() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string } & CreateNICInput) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<NIC>(`/system-containers/${id}/nics`, body));
    },
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useDeleteNIC() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; nicId: string }) =>
      assertSuccess(await api.del<null>(`/system-containers/${input.id}/nics/${input.nicId}`)),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useAddMount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string } & CreateMountInput) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Mount>(`/system-containers/${id}/mounts`, body));
    },
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useDeleteMount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; mountId: string }) =>
      assertSuccess(await api.del<null>(`/system-containers/${input.id}/mounts/${input.mountId}`)),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useCreateSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; name: string; description?: string; freeze?: boolean }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Snapshot>(`/system-containers/${id}/snapshots`, body));
    },
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useRevertSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; snapshotId: string }) =>
      assertSuccess(await api.post<null>(`/system-containers/${input.id}/snapshots/${input.snapshotId}/revert`, {})),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}

export function useDeleteSnapshot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; snapshotId: string }) =>
      assertSuccess(await api.del<null>(`/system-containers/${input.id}/snapshots/${input.snapshotId}`)),
    onSuccess: (_data, input) => invalidate(queryClient, input.id),
  });
}
