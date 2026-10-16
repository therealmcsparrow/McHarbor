// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';

export type CapacityCrossing = {
  percent: number;
  days: number;
  at: string;
};

export type CapacityRefusal =
  | ''
  | 'no_fit'
  | 'short_series'
  | 'noisy'
  | 'not_rising'
  | 'already_over'
  | 'beyond_horizon';

export type CapacitySubject = {
  environmentId: string;
  subjectKind: 'rootfs' | 'storage_location';
  subjectId: string;
  subjectName: string;
  usedBytes: number;
  totalBytes: number;
  usedPercent: number;
  observedDays: number;
  requiredDays: number;
  samples: number;
  windowDays: number;
  percentPerDay: number;
  r2: number;
  amber?: CapacityCrossing;
  red?: CapacityCrossing;
  refusal?: CapacityRefusal;
};

export type CapacityReport = {
  subjects: CapacitySubject[];
  windowDays: number;
  requiredDays: number;
  amberPercent: number;
  redPercent: number;
  bucketHours: number;
};

export type IdleResource = {
  environmentId: string;
  containerId: string;
  containerName: string;
  reason: 'stopped' | 'quiet';
  idleDays: number;
  sizeKnown: boolean;
  sizeReason?: 'agent_environment' | 'not_sampled';
  sizeRwBytes: number;
  sizeRootBytes: number;
};

export type IdleReport = {
  containers: IdleResource[];
  thresholdDays: number;
  reclaimableBytes: number;
  sizesUnavailable: boolean;
};

export function useCapacity(envId?: string, enabled = true) {
  const currentId = useEnvironmentStore((state) => state.currentId);
  const target = envId ?? currentId;

  return useQuery({
    queryKey: ['capacity', target],
    queryFn: () => api.get<CapacityReport>('/metrics/capacity', target ? { env: target } : {}).then(assertSuccess),
    staleTime: 5 * 60_000,
    enabled,
  });
}

export function useIdleResources(envId?: string, days?: number, enabled = true) {
  const currentId = useEnvironmentStore((state) => state.currentId);
  const target = envId ?? currentId;

  return useQuery({
    queryKey: ['capacity-idle', target, days ?? null],
    queryFn: () => {
      const params: Record<string, string> = {};
      if (target) params.env = target;
      if (days) params.days = String(days);
      return api.get<IdleReport>('/metrics/capacity/idle', params).then(assertSuccess);
    },
    staleTime: 5 * 60_000,
    enabled,
  });
}
