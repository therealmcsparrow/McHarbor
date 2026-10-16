// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { DesiredStateKind, DriftClass } from './useReconciler';

export type DriftReportRow = {
  desiredStateId: string;
  name: string;
  kind: DesiredStateKind;
  envId: string;
  class: DriftClass;
  field: string;
  expected: string;
  actual: string;
  firstSeenAt: string;
  checkedAt: string;
};

export function useDriftReport(driftClass: DriftClass | '', enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['reconciler', 'report', envId, driftClass],
    queryFn: () =>
      api
        .get<DriftReportRow[]>('/reconciler/report', {
          ...(envId ? { env: envId } : {}),
          ...(driftClass ? { class: driftClass } : {}),
        })
        .then(assertSuccess),
    refetchInterval: 30_000,
    enabled,
  });
}
