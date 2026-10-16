// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ImportListView } from '../types';
import { isImportRunning, useImportJob } from '../importJob';

const KEYS = {
  list: ['vms', 'imports'] as const,
};

export function useImports() {
  const job = useImportJob();
  const jobRunning = isImportRunning(job.phase);

  return useQuery({
    queryKey: KEYS.list,
    queryFn: async () => assertSuccess(await api.get<ImportListView>('/vms/imports')),
    refetchInterval: (query) => {
      const data = query.state.data as ImportListView | undefined;
      const busy = data?.imports?.some((record) => record.state === 'inspecting' || record.state === 'converting');
      return busy || jobRunning ? 3_000 : false;
    },
  });
}

export function useDeleteImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (importId: string) => assertSuccess(await api.del(`/vms/imports/${importId}`)),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: KEYS.list });
    },
  });
}

export function useCancelImport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (importId: string) => assertSuccess(await api.post(`/vms/imports/${importId}/cancel`, {})),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: KEYS.list });
    },
  });
}
