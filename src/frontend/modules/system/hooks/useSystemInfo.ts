// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { SystemInfo } from '../types';

export function useSystemInfo() {
  return useQuery({
    queryKey: ['system-info'],
    queryFn: () => api.get<SystemInfo>('/about').then((r) => r.data),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
}
