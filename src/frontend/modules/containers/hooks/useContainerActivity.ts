// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import type { ImageHistoryItem } from '@core/types/docker';
import { useAuditLogs } from '@resources/hooks/useAuditLogs';
import { useImageHistory } from '@resources/hooks/useImages';
import { useContainerLogs } from '@resources/hooks/useContainerLogs';
import { useContainerProcesses } from './useContainerProcesses';
import { createSnapshotLogRecords } from '@resources/components/logs/logRecords';

export type ContainerActivitySource = 'all' | 'logs' | 'history' | 'changes' | 'processes';
export type ContainerActivitySeverity = 'debug' | 'info' | 'warn' | 'error';

export type ContainerActivityItem = {
  id: string;
  source: Exclude<ContainerActivitySource, 'all'>;
  severity: ContainerActivitySeverity;
  timestamp: string;
  title: string;
  description?: string;
};

function historyTimestamp(item: ImageHistoryItem) {
  return new Date(item.Created * 1000).toISOString();
}

function auditSeverity(action: string): ContainerActivitySeverity {
  if (/kill|remove|delete|fail|error/i.test(action)) return 'error';
  if (/stop|pause|restart|recreate/i.test(action)) return 'warn';
  return 'info';
}

export function useContainerActivity(
  containerId: string,
  containerImage: string,
  isRunning: boolean,
  source: ContainerActivitySource,
) {
  const includes = useCallback(
    (candidate: ContainerActivitySource) => source === 'all' || source === candidate,
    [source],
  );
  const logs = useContainerLogs(containerId, 500, false, includes('logs'));
  const audit = useAuditLogs('', 'container', { from: null, to: null }, containerId, includes('changes'));
  const imageHistory = useImageHistory(containerImage, includes('history'));
  const processes = useContainerProcesses(containerId, isRunning && includes('processes'), source === 'processes');

  const items = useMemo(() => {
    const next: ContainerActivityItem[] = [];
    if (includes('logs')) {
      for (const record of createSnapshotLogRecords(logs.data ?? '')) {
        next.push({
          id: `log-${record.id}`,
          source: 'logs',
          severity: ['debug', 'warn', 'error'].includes(record.level)
            ? (record.level as ContainerActivitySeverity)
            : 'info',
          timestamp: record.timestamp,
          title: record.message,
          description: record.stream,
        });
      }
    }
    if (includes('changes')) {
      for (const page of audit.data?.pages ?? []) {
        for (const entry of page.items) {
          next.push({
            id: `change-${entry.id}`,
            source: 'changes',
            severity: auditSeverity(entry.action),
            timestamp: entry.timestamp || entry.createdAt,
            title: entry.action,
            description: [entry.username, entry.details].filter(Boolean).join(' · '),
          });
        }
      }
    }
    if (includes('history')) {
      for (const entry of imageHistory.data ?? []) {
        next.push({
          id: `history-${entry.Id || entry.Created}-${entry.CreatedBy}`,
          source: 'history',
          severity: 'info',
          timestamp: historyTimestamp(entry),
          title: entry.CreatedBy || entry.Comment,
          description: entry.Comment || undefined,
        });
      }
    }
    if (isRunning && includes('processes') && processes.data) {
      const timestamp = new Date(processes.dataUpdatedAt).toISOString();
      for (const process of processes.data.Processes) {
        next.push({
          id: `process-${process.join('\u0000')}`,
          source: 'processes',
          severity: 'info',
          timestamp,
          title: process.at(-1) || process.join(' '),
          description: process.slice(0, -1).join(' · '),
        });
      }
    }
    return next
      .filter((item) => !Number.isNaN(Date.parse(item.timestamp)))
      .sort((left, right) => Date.parse(right.timestamp) - Date.parse(left.timestamp));
  }, [audit.data?.pages, imageHistory.data, logs.data, processes.data, processes.dataUpdatedAt, isRunning, includes]);

  const logsAvailable = logs.dataUpdatedAt > 0;
  const auditAvailable = audit.dataUpdatedAt > 0;
  const historyAvailable = imageHistory.dataUpdatedAt > 0;
  const processesAvailable = processes.dataUpdatedAt > 0;
  const isLoading =
    (includes('logs') && logs.isLoading) ||
    (includes('changes') && audit.isLoading) ||
    (includes('history') && imageHistory.isLoading) ||
    (isRunning && includes('processes') && processes.isLoading);
  const sourceError =
    (includes('logs') && !logsAvailable ? logs.error : null) ??
    (includes('changes') && !auditAvailable ? audit.error : null) ??
    (includes('history') && !historyAvailable ? imageHistory.error : null) ??
    (isRunning && includes('processes') && !processesAvailable ? processes.error : null);
  const hasAvailableSource =
    (includes('logs') && logsAvailable) ||
    (includes('changes') && auditAvailable) ||
    (includes('history') && historyAvailable) ||
    (isRunning && includes('processes') && processesAvailable);

  return {
    items,
    isLoading,
    error: source === 'all' ? (!isLoading && !hasAvailableSource ? sourceError : null) : sourceError,
    refetch: () => {
      const requests: Array<Promise<unknown>> = [];
      if (includes('logs')) requests.push(logs.refetch());
      if (includes('changes')) requests.push(audit.refetch());
      if (includes('history')) requests.push(imageHistory.refetch());
      if (isRunning && includes('processes')) requests.push(processes.refetch());
      return Promise.all(requests);
    },
  };
}
