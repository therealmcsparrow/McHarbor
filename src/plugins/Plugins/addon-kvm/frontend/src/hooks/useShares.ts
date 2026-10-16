// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface Share {
  id: string;
  vmId: string;
  tag: string;
  hostPath: string;
  readOnly: boolean;
  mountCommand?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ShareRequest {
  tag: string;
  hostPath: string;
  readOnly: boolean;
}

export function useShares(vmId: string) {
  return useQuery({
    queryKey: ['kvm', 'shares', vmId],
    queryFn: async () => assertSuccess(await api.get<Share[]>(`/vms/${vmId}/shares`)),
  });
}

export function useShareRoots(environmentId?: string) {
  return useQuery({
    queryKey: ['kvm', 'share-roots', environmentId],
    enabled: Boolean(environmentId),
    queryFn: async () => assertSuccess(await api.get<{ roots: string[] }>(`/vms/share-roots?env=${environmentId}`)),
  });
}

export function useAddShare(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (req: ShareRequest) => assertSuccess(await api.post<Share>(`/vms/${vmId}/shares`, req)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['kvm', 'shares', vmId] });
    },
  });
}

export function useRemoveShare(vmId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (shareId: string) => assertSuccess(await api.del(`/vms/${vmId}/shares/${shareId}`)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['kvm', 'shares', vmId] });
    },
  });
}
