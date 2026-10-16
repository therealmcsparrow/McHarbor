// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/** Windows the usage endpoint accepts, matching the resource-history grammar. */
export type UsageRange = '1h' | '6h' | '12h' | '24h' | '7d' | '30d' | '90d';

/** What the range control offers. The endpoint accepts more, for API clients. */
export const USAGE_RANGES: UsageRange[] = ['1h', '6h', '24h', '7d', '30d'];

export type APIKeyUsagePoint = {
  timestamp: string;
  count: number;
};

export type APIKeyUsageSeries = {
  apiKeyId: string;
  from: string;
  to: string;
  /** What the stored data actually spans; absent when there is none. */
  coveredFrom?: string;
  coveredTo?: string;
  bucketSeconds: number;
  total: number;
  successCount: number;
  clientErrorCount: number;
  serverErrorCount: number;
  averageDurationMs: number;
  uniqueClients: number;
  points: APIKeyUsagePoint[];
};

/**
 * One recorded request. Metadata only — there is deliberately no header,
 * body, query string or token anywhere in this shape.
 */
export type APIKeyUsageEvent = {
  id: string;
  apiKeyId: string;
  occurredAt: string;
  method: string;
  routePattern: string;
  statusCode: number;
  durationMs: number;
  authOutcome: 'accepted' | 'revoked' | 'expired' | 'owner_invalid';
  environmentId: string | null;
  peerIp: string;
  clientIp: string;
  clientIpSource: string;
  userAgent: string;
};

export type UsageEventFilters = {
  page: number;
  perPage: number;
  method?: string;
  statusClass?: string;
  authOutcome?: string;
  environmentId?: string;
};

type PaginatedEvents = {
  items: APIKeyUsageEvent[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
};

/**
 * The request-count series behind the graph.
 *
 * Polled at the recorder's own cadence rather than faster: the writer batches
 * on a sub-second timer but the chart's buckets are minutes wide, so a
 * quicker poll would redraw identical bars.
 */
export function useAPIKeyUsageSeries(id: string | undefined, range: UsageRange) {
  return useQuery({
    queryKey: ['api-keys', 'usage', id, range],
    queryFn: () => api.get<APIKeyUsageSeries>(`/api-keys/${id}/usage`, { range }).then(assertSuccess),
    enabled: !!id,
    refetchInterval: 30_000,
  });
}

/** One page of the request history, filtered server-side. */
export function useAPIKeyUsageEvents(id: string | undefined, filters: UsageEventFilters) {
  const params: Record<string, string> = {
    page: String(filters.page),
    per_page: String(filters.perPage),
  };
  if (filters.method) params.method = filters.method;
  if (filters.statusClass) params.status_class = filters.statusClass;
  if (filters.authOutcome) params.auth_outcome = filters.authOutcome;
  if (filters.environmentId) params.environment_id = filters.environmentId;

  return useQuery({
    queryKey: ['api-keys', 'usage-events', id, filters],
    queryFn: () => api.get<PaginatedEvents>(`/api-keys/${id}/usage/events`, params).then(assertSuccess),
    enabled: !!id,
    refetchInterval: 30_000,
  });
}

// The manual clear lives with the other retention purges in
// modules/settings/hooks/useRetentionSettings.ts, because it is a retention
// action on the whole install rather than an action on any one key.
