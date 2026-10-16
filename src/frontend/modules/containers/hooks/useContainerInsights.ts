// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useNow } from './useNow';
import type { ContainerInspect } from '@core/types/docker';
import type { DetailTabId } from '../components/ContainerDetailTabs';
import type { UseContainerStatsResult } from './useContainerStats';
import { useContainerBackupRuns } from '@resources/hooks/useContainerBackups';
import { useContainerUpdateResults } from './useContainerUpdates';
import { useImageScans } from './useScans';

export type ContainerInsight = {
  id: string;
  severity: 'info' | 'warning' | 'error';
  titleKey: string;
  descriptionKey: string;
  values?: Record<string, string | number>;
  target: DetailTabId;
};

export function useContainerInsights(container: ContainerInspect, stats?: UseContainerStatsResult) {
  const backupRuns = useContainerBackupRuns(container.Id);
  const updateResults = useContainerUpdateResults();
  const { data: scans } = useImageScans(container.Config?.Image ?? container.Image);
  const now = useNow();

  return useMemo(() => {
    const insights: ContainerInsight[] = [];
    const recent = stats?.history.slice(-10) ?? [];
    const average = (values: number[]) =>
      values.length > 0 ? values.reduce((sum, value) => sum + value, 0) / values.length : 0;
    const averageCPU = average(recent.map((metric) => metric.cpuPercent));
    const averageMemory = average(recent.map((metric) => metric.memPercent));
    const latestScan = scans?.items?.[0];
    const lastBackup = (backupRuns.data ?? []).reduce<string | null>((latest, run) => {
      if (run.operation !== 'backup' || run.status !== 'success') {
        return latest;
      }
      const timestamp = run.completedAt || run.startedAt;
      return !latest || Date.parse(timestamp) > Date.parse(latest) ? timestamp : latest;
    }, null);

    if (container.State?.Health?.Status === 'unhealthy') {
      insights.push({
        id: 'unhealthy',
        severity: 'error',
        titleKey: 'insights.unhealthyTitle',
        descriptionKey: 'insights.unhealthyDescription',
        values: { count: container.State.Health.FailingStreak },
        target: 'overview',
      });
    }
    if (container.State?.OOMKilled) {
      insights.push({
        id: 'oom',
        severity: 'error',
        titleKey: 'insights.oomTitle',
        descriptionKey: 'insights.oomDescription',
        target: 'resources',
      });
    }
    if ((latestScan?.criticalCount ?? 0) > 0) {
      insights.push({
        id: 'vulnerabilities',
        severity: 'error',
        titleKey: 'insights.vulnerabilityTitle',
        descriptionKey: 'insights.vulnerabilityDescription',
        values: { count: latestScan?.criticalCount ?? 0 },
        target: 'security',
      });
    }
    if (averageCPU >= 80) {
      insights.push({
        id: 'cpu',
        severity: 'warning',
        titleKey: 'insights.cpuTitle',
        descriptionKey: 'insights.cpuDescription',
        values: { percent: averageCPU.toFixed(1) },
        target: 'resources',
      });
    }
    if (averageMemory >= 85) {
      insights.push({
        id: 'memory',
        severity: 'warning',
        titleKey: 'insights.memoryTitle',
        descriptionKey: 'insights.memoryDescription',
        values: { percent: averageMemory.toFixed(1) },
        target: 'resources',
      });
    }
    if ((container.RestartCount ?? 0) >= 3) {
      insights.push({
        id: 'restarts',
        severity: 'warning',
        titleKey: 'insights.restartTitle',
        descriptionKey: 'insights.restartDescription',
        values: { count: container.RestartCount },
        target: 'activity',
      });
    }
    if (container.HostConfig?.Privileged) {
      insights.push({
        id: 'privileged',
        severity: 'warning',
        titleKey: 'insights.privilegedTitle',
        descriptionKey: 'insights.privilegedDescription',
        target: 'security',
      });
    }
    if (backupRuns.isSuccess && (!lastBackup || now - Date.parse(lastBackup) > 7 * 24 * 60 * 60 * 1000)) {
      insights.push({
        id: 'backup',
        severity: 'warning',
        titleKey: 'insights.backupTitle',
        descriptionKey: 'insights.backupDescription',
        target: 'backups',
      });
    }
    if (updateResults?.get(container.Id)?.updateAvailable) {
      insights.push({
        id: 'update',
        severity: 'info',
        titleKey: 'insights.updateTitle',
        descriptionKey: 'insights.updateDescription',
        target: 'updates',
      });
    }
    if (!container.Config?.Healthcheck?.Test?.length) {
      insights.push({
        id: 'healthcheck',
        severity: 'info',
        titleKey: 'insights.healthcheckTitle',
        descriptionKey: 'insights.healthcheckDescription',
        target: 'environment',
      });
    }
    return insights.slice(0, 6);
  }, [backupRuns.data, backupRuns.isSuccess, container, now, scans?.items, stats?.history, updateResults]);
}
