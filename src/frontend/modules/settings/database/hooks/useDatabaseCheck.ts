// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { ACTIVE_CHECK_PHASES, type DatabaseCheckStatus, type RepairResponse } from '../types';

export function useCheckStatus(enabled: boolean) {
  return useQuery({
    queryKey: ['database', 'check'],
    queryFn: () => api.get<DatabaseCheckStatus>('/database/check').then((r) => (r.success ? (r.data ?? null) : null)),
    enabled,
    retry: false,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      return ACTIVE_CHECK_PHASES.includes(data.phase) ? 1_000 : false;
    },
  });
}

export function useStartCheck() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<DatabaseCheckStatus>('/database/check', {}).then(assertSuccess),
    onMutate: () => {
      queryClient.setQueryData(['database', 'check'], null);
    },
    onSuccess: (currentStatus) => {
      queryClient.setQueryData(['database', 'check'], currentStatus);
    },
  });
}

export function useCancelCheck() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post('/database/check/cancel', {}).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['database', 'check'] });
    },
  });
}

export function useRepair() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (findingIds: string[]) =>
      api.post<RepairResponse>('/database/check/repair', { findingIds }).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['database', 'check'] });
      void queryClient.invalidateQueries({ queryKey: ['database', 'config'] });
    },
  });
}
