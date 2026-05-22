// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { AuditEntry } from '../components/AuditEntryDetails';

export function useAuditLogs(action: string, entityType: string) {
  const envId = useEnvironmentStore((state) => state.currentId);
  const params: Record<string, string> = { per_page: '100' };

  if (action) params.action = action;
  if (entityType) params.entity_type = entityType;
  if (envId) params.env = envId;

  return useQuery({
    queryKey: ['audit-logs', envId, action, entityType],
    queryFn: () =>
      api
        .get<PaginatedData<AuditEntry>>('/audit', params)
        .then((response) => response.data?.items ?? []),
    refetchInterval: 30_000,
  });
}
