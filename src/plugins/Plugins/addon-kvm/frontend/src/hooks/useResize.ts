// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface ResizeResult {
  requestedMemoryMb?: number;
  actualMemoryMb?: number;
  blockSizeMb?: number;
  requestedVcpus?: number;
  actualVcpus?: number;
  /** False when the guest did not reach the requested figure. Not an error. */
  complied: boolean;
}

export interface ThrottleRequest {
  readIops: number;
  writeIops: number;
  readBps: number;
  writeBps: number;
}

export function useResizeMemory(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (memoryMb: number) =>
      assertSuccess(await api.post<ResizeResult>(`/vms/${vmId}/memory`, { memoryMb })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['kvm', 'vm', vmId] });
    },
  });
}

export function useSetVCPUs(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vcpus: number) => assertSuccess(await api.post<ResizeResult>(`/vms/${vmId}/vcpus`, { vcpus })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['kvm', 'vm', vmId] });
    },
  });
}

export function useSetThrottle(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ diskId, limits }: { diskId: string; limits: ThrottleRequest }) =>
      assertSuccess(await api.put(`/vms/${vmId}/disks/${diskId}/throttle`, limits)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['kvm', 'vm', vmId] });
    },
  });
}

export function useInspectImage(vmId: string) {
  return useMutation({
    mutationFn: async ({
      diskId,
      operation,
      repair,
    }: {
      diskId: string;
      operation: 'info' | 'check' | 'map' | 'measure';
      repair?: boolean;
    }) =>
      assertSuccess(
        await api.post<unknown>(`/vms/${vmId}/disks/${diskId}/inspect`, {
          operation,
          repair: repair ?? false,
        }),
      ),
  });
}
