// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export interface PoolMovePlan {
  poolId: string;
  from: string;
  to: string;
  entries: number;
  bytes: number;
  runningGuests: string[];
}

export interface PoolMoveStatus {
  poolId: string;
  state: 'idle' | 'running' | 'done' | 'failed';
  percent: number;
  bytesMoved: number;
  bytesTotal: number;
  moved: number;
  failures?: string[];
  error?: string;
  stoppedGuests?: string[];
}

export interface RepairResult {
  entries: number;
  moved: { kind: string; from: string; to: string }[];
  failures?: { path: string; reason: string }[];
  notes?: string[];
}

const KEYS = {
  plan: (poolId: string, target: string) => ['compute', 'pools', poolId, 'move', 'plan', target] as const,
  status: (poolId: string) => ['compute', 'pools', poolId, 'move', 'status'] as const,
};

export function usePoolMovePlan(poolId: string, target: string) {
  return useQuery({
    queryKey: KEYS.plan(poolId, target),
    enabled: Boolean(poolId && target),
    staleTime: 0,
    queryFn: async () =>
      assertSuccess(
        await api.post<PoolMovePlan>(`/compute/pools/${poolId}/move/plan`, {
          target,
        }),
      ),
  });
}

export function usePoolMoveStatus(poolId: string, active: boolean) {
  return useQuery({
    queryKey: KEYS.status(poolId),
    enabled: active && Boolean(poolId),
    refetchInterval: active ? 1000 : false,
    queryFn: async () => assertSuccess(await api.get<PoolMoveStatus>(`/compute/pools/${poolId}/move`)),
  });
}

export function useStartPoolMove() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { poolId: string; target: string }) =>
      assertSuccess(
        await api.post<PoolMoveStatus>(`/compute/pools/${input.poolId}/move`, {
          target: input.target,
        }),
      ),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'pools'] });
    },
  });
}

export function useRepairPool() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (poolId: string) =>
      assertSuccess(await api.post<RepairResult>(`/compute/pools/${poolId}/repair`, {})),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'pools'] });
    },
  });
}
