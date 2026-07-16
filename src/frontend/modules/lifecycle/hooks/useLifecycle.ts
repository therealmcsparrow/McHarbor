// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type LifecycleSubjectType = 'container' | 'image' | 'volume' | 'network' | 'stack';

export type LifecycleSeverity = 'info' | 'success' | 'warning' | 'error';

export type LifecycleEvent = {
  id: string;
  environmentId?: string;
  subjectType: LifecycleSubjectType;
  subjectId: string;
  subjectName?: string;
  eventType: string;
  action: string;
  state?: string;
  severity: LifecycleSeverity;
  metadata?: string;
  source: 'docker' | 'compose' | 'mcharbor';
  timestamp: string;
};

export type LifecycleListParams = {
  page?: number;
  perPage?: number;
  envId?: string;
  subjectType?: LifecycleSubjectType | '';
  severity?: LifecycleSeverity | '';
  from?: string;
  until?: string;
  search?: string;
};

export type LifecycleListResponse = {
  items: LifecycleEvent[];
  total: number;
  page: number;
  perPage: number;
};

export function useLifecycleEvents(params: LifecycleListParams = {}) {
  const search = new URLSearchParams();
  if (params.page) search.set('page', String(params.page));
  if (params.perPage) search.set('perPage', String(params.perPage));
  if (params.envId) search.set('envId', params.envId);
  if (params.subjectType) search.set('subjectType', params.subjectType);
  if (params.severity) search.set('severity', params.severity);
  if (params.from) search.set('from', params.from);
  if (params.until) search.set('until', params.until);
  if (params.search) search.set('search', params.search);
  const qs = search.toString();
  return useQuery({
    queryKey: ['lifecycle', params],
    queryFn: () =>
      api
        .get<LifecycleListResponse>(`/lifecycle${qs ? `?${qs}` : ''}`)
        .then((r) => r.data),
    refetchInterval: 15_000,
  });
}

export function useRecentLifecycleEvents(n = 10) {
  return useQuery({
    queryKey: ['lifecycle', 'recent', n],
    queryFn: () =>
      api
        .get<{ items: LifecycleEvent[] }>(`/lifecycle/recent?n=${n}`)
        .then((r) => r.data),
    refetchInterval: 10_000,
  });
}
