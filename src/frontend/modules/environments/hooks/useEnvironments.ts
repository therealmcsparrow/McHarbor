// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { HostMetrics } from '@core/types/docker';

type EnvironmentInfo = {
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
  timezone: string;
  agentStatus?: string;
  agentHostname?: string;
  agentOs?: string;
  agentArch?: string;
  agentVersion?: string;
  agentLastSeen?: string;
};

type DashboardStats = {
  containers: { total: number; running: number; stopped: number; paused: number };
  images: number;
  volumes: number;
  networks: number;
  cpuHistory?: Array<{ timestamp: string; value: number }>;
  memoryHistory?: Array<{ timestamp: string; value: number }>;
  networkRxHistory?: Array<{ timestamp: string; value: number }>;
  networkTxHistory?: Array<{ timestamp: string; value: number }>;
  blockReadHistory?: Array<{ timestamp: string; value: number }>;
  blockWriteHistory?: Array<{ timestamp: string; value: number }>;
};

export type { EnvironmentInfo, DashboardStats };

export function useEnvironment(id: string) {
  return useQuery({
    queryKey: ['environments', id],
    queryFn: () =>
      api.get<EnvironmentInfo>(`/environments/${id}`).then((r) => r.data),
    enabled: !!id,
  });
}

export function useEnvironmentMetrics(envId: string, enabled = true) {
  return useQuery({
    queryKey: ['dashboard-stats', envId],
    queryFn: () =>
      api.get<DashboardStats>('/dashboard/stats', { env: envId }).then((r) => r.data),
    refetchInterval: enabled ? 15_000 : false,
    enabled: enabled && !!envId,
  });
}

export function useEnvironmentHostMetrics(envId: string) {
  return useQuery({
    queryKey: ['host-metrics', envId],
    queryFn: () =>
      api.get<HostMetrics>('/metrics/host', { env: envId }).then((r) => r.data),
    refetchInterval: 30_000,
    enabled: !!envId,
  });
}

