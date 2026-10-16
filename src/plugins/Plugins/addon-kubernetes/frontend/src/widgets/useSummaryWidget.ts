// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { clusterParams, useSelectedCluster } from '../cluster-store';
import { list } from '../format';
import { stateTone } from '../components/kubernetes-facets';
import type { KubernetesSummary, Row } from '../types';

/** Base path of the plugin's pages, for click-throughs from a tile. */
export const PLUGIN_PATH = '/plugins/addon-kubernetes';

/**
 * The same summary the pages read, on the widgets' shared QueryClient. A
 * slower interval than the page: a dashboard is glanced at, not watched.
 */
export function useSummaryWidget() {
  const clusterId = useSelectedCluster();
  return useQuery({
    queryKey: ['kubernetes', 'summary', clusterId],
    queryFn: () => api.get<KubernetesSummary>('/kubernetes/summary', clusterParams(clusterId)).then(assertSuccess),
    refetchInterval: 30_000,
  });
}

/** `a/b` → { ready: a, total: b }; anything else → zeros. */
export function readyParts(row: Row): { ready: number; total: number } {
  const [a, b] = list(row.ready).split('/');
  const ready = Number(a);
  const total = Number(b);
  if (!Number.isFinite(ready) || !Number.isFinite(total)) return { ready: 0, total: 0 };
  return { ready, total };
}

export type PodBuckets = { running: number; pending: number; failed: number; succeeded: number; other: number };

export function podBuckets(pods: Row[]): PodBuckets {
  const out: PodBuckets = { running: 0, pending: 0, failed: 0, succeeded: 0, other: 0 };
  for (const pod of pods) {
    const statusValue = list(pod.status);
    if (statusValue === 'Running') out.running += 1;
    else if (statusValue === 'Succeeded') out.succeeded += 1;
    else if (stateTone(statusValue) === 'danger') out.failed += 1;
    else if (stateTone(statusValue) === 'warning') out.pending += 1;
    else out.other += 1;
  }
  return out;
}
