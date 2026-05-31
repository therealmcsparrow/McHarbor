// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type EnvironmentListItem = {
  id: string;
  name: string;
  orchestratorType: 'docker' | 'kubernetes';
  connectionType: string;
  socketPath?: string;
  host?: string;
  port?: number;
  dockerVersion: string | null;
  k8sVersion: string | null;
  isDefault: boolean;
  isActive: boolean;
  lastConnected: string | null;
  scheduledUpdateCheckEnabled: boolean;
  automaticImagePruningEnabled: boolean;
  trackContainerEventsEnabled: boolean;
  collectContainerMetricsEnabled: boolean;
  highlightContainerChangesEnabled: boolean;
  dockerDiskUsageNotificationsEnabled: boolean;
  dockerDiskUsageThresholdPercent: number;
  agentStatus?: string;
  agentHostname?: string;
  agentVersion?: string;
};

export function useEnvironmentList() {
  return useQuery({
    queryKey: ['environments'],
    queryFn: () =>
      api.get<EnvironmentListItem[]>('/environments').then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}
