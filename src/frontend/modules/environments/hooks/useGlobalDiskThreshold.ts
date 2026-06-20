// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

const DEFAULT_DISK_THRESHOLD = 80;

type SettingResponse = {
  key: string;
  value: string;
};

export function useGlobalDiskThresholdPercent() {
  return useQuery({
    queryKey: ['settings', 'disk_usage_default_threshold_percent'],
    queryFn: async () => {
      try {
        const response = await api.get<SettingResponse>('/settings/disk_usage_default_threshold_percent');
        const value = response.data?.value;
        if (!value) {
          return DEFAULT_DISK_THRESHOLD;
        }
        const parsed = Number.parseInt(value, 10);
        if (!Number.isFinite(parsed) || parsed < 1 || parsed > 100) {
          return DEFAULT_DISK_THRESHOLD;
        }
        return parsed;
      } catch {
        return DEFAULT_DISK_THRESHOLD;
      }
    },
    staleTime: 5 * 60 * 1000,
  });
}
