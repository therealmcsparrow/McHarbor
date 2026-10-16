// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

export type EnvironmentListItem = {
  id: string;
  name: string;
  connectionType: string;
  socketPath?: string;
  host?: string;
  port?: number;
  dockerVersion: string | null;
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
  agentVersion?: string;
  agentBuild?: string;
  agentHostIp?: string;
  agentTlsMode?: string;
  pendingConnectionType?: string;
  pendingMigrationStartedAt?: string;
  migrationRevertTo?: string;
  agentOs?: string;
  agentArch?: string;
  agentLastSeen?: string;
  agentInstallKind?: string;
  sshHost?: string;
  sshPort?: number;
  sshUser?: string;
  createdAt: string;
  updatedAt: string;
  logRetentionDays: number;
  metricRetentionHours: number;
  autoUpdateWindowStart: string;
  autoUpdateWindowEnd: string;
  containerPruneEnabled: boolean;
  containerPruneStoppedDays: number;
  imagePruneDanglingOnly: boolean;
  stackCount?: number;
};

export function useEnvironmentList() {
  return useQuery({
    queryKey: ['environments'],
    queryFn: () => api.get<EnvironmentListItem[]>('/environments').then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}
