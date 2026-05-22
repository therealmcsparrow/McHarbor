// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export function useNodeCatalogAvailability() {
  return useQuery({
    queryKey: ['workflow-nodes', 'availability'],
    queryFn: () => api.get<Record<string, boolean>>('/workflow-nodes').then(assertSuccess),
    staleTime: 5 * 60_000,
  });
}

export function useUpdateNodeCatalogEnabled() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) =>
      api.put<{ key: string; enabled: boolean }>(`/workflow-nodes/${key}`, { enabled }).then(assertSuccess),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['workflow-nodes', 'availability'] });
    },
  });
}
