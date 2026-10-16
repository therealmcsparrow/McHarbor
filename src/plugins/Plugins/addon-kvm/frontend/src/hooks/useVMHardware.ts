// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { Disk, NIC } from '../types';
import { invalidateVM } from './vmQueryKeys';

export function useAddDisk() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      poolId: string;
      poolBackend: string;
      poolTarget: string;
      sizeGb: number;
      bus?: string;
    }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Disk>(`/vms/${id}/disks`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useChangeDiskBus() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; diskId: string; bus: string }) =>
      assertSuccess(await api.patch<Disk>(`/vms/${input.id}/disks/${input.diskId}`, { bus: input.bus })),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

/**
 * A disk's shared access and its place in the boot order.
 *
 * One mutation for both because they travel on the same PATCH and are always
 * edited from the same row; splitting them would double the invalidations for
 * no gain.
 */
export function useUpdateDiskFlags() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; diskId: string; shareRw?: boolean; bootIndex?: number }) =>
      assertSuccess(
        await api.patch<Disk>(`/vms/${input.id}/disks/${input.diskId}`, {
          ...(input.shareRw === undefined ? {} : { shareRw: input.shareRw }),
          ...(input.bootIndex === undefined ? {} : { bootIndex: input.bootIndex }),
        }),
      ),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useResizeDisk() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; diskId: string; sizeGb: number }) =>
      assertSuccess(await api.patch<Disk>(`/vms/${input.id}/disks/${input.diskId}`, { sizeGb: input.sizeGb })),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useDeleteDisk() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; diskId: string }) =>
      assertSuccess(await api.del<null>(`/vms/${input.id}/disks/${input.diskId}`)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useAddNIC() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      networkId: string;
      bridge: string;
      vlanTag?: number;
      attachKind?: string;
      parentLink?: string;
      linkMode?: string;
      model?: string;
    }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<NIC>(`/vms/${id}/nics`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useUpdateNIC() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; nicId: string; linkUp?: boolean; model?: string }) => {
      const { id, nicId, ...body } = input;
      return assertSuccess(await api.patch<NIC>(`/vms/${id}/nics/${nicId}`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useDeleteNIC() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; nicId: string }) =>
      assertSuccess(await api.del<null>(`/vms/${input.id}/nics/${input.nicId}`)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useAttachMedia() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; sourcePath: string; imageId?: string; label?: string }) => {
      const { id, ...body } = input;
      return assertSuccess(await api.post<Disk>(`/vms/${id}/media`, body));
    },
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useAttachGuestAgentDisc() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string }) =>
      assertSuccess(await api.post<Disk>(`/vms/${input.id}/guest-agent-disc`, {})),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}

export function useEjectMedia() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { id: string; diskId: string }) =>
      assertSuccess(await api.del<null>(`/vms/${input.id}/media/${input.diskId}`)),
    onSuccess: (_data, input) => invalidateVM(queryClient, input.id),
  });
}
