// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type EnvironmentStackSummary = {
  id: string;
  name: string;
  status: string;
  services: Array<{
    name: string;
    containerId?: string;
    status: string;
    image: string;
  }>;
  description?: string;
  type: 'managed' | 'discovered';
};

export function useStacksForEnvironment(envId?: string, enabled = true) {
  return useQuery({
    queryKey: ['stacks', 'scope', envId],
    queryFn: () =>
      api
        .get<EnvironmentStackSummary[]>('/stacks', envId ? { env: envId } : {})
        .then((response) => response.data ?? []),
    enabled: enabled && !!envId,
  });
}
