// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type AgentLogEntry = {
  id: string;
  level: 'debug' | 'info' | 'warn' | 'error' | (string & {});
  message: string;
  attrs?: Record<string, unknown>;
  loggedAt: string;
};

export type AgentLogsPage = PaginatedData<AgentLogEntry>;

/**
 * The remote agent's own log, replayed from the buffer it kept while it could
 * not reach McHarbor. Newest first. Refetched on a slow cadence: the rows only
 * change when an agent reconnects, and an operator looking at this card is
 * usually waiting for exactly that.
 */
export function useAgentLogs(envId: string, page: number, perPage = 25, enabled = true, live = false) {
  return useQuery({
    queryKey: ['agents', envId, 'logs', page, perPage],
    queryFn: async () => {
      const response = await api.get<AgentLogsPage>(`/agents/${envId}/logs`, {
        page: String(page),
        per_page: String(perPage),
      });
      return assertSuccess(response);
    },
    enabled: enabled && envId !== '',
    refetchInterval: live ? 5_000 : 30_000,
    placeholderData: (previous) => previous,
  });
}
