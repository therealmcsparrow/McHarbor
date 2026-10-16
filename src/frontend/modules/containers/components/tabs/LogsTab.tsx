// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { useContainerLogs } from '@resources/hooks/useContainerLogs';
import type { ContainerInspect } from '@core/types/docker';
import { LogFilterBar } from '@resources/components/logs/LogFilterBar';
import {
  createSnapshotLogRecords,
  detectLogLevel,
  downloadLogLines,
  type LogDisplayRecord,
} from '@resources/components/logs/logRecords';
import { useLogFilter } from '@resources/components/logs/useLogFilter';
import { LogContextDrawer } from '../LogContextDrawer';

type LogsTabProps = {
  container: ContainerInspect;
  isRunning: boolean;
  active: boolean;
};

export function LogsTab({ container, isRunning, active }: LogsTabProps) {
  const containerId = container.Id;
  const { t } = useTranslation('containers');
  const [live, setLive] = useState(false);
  const [selectedRecord, setSelectedRecord] = useState<LogDisplayRecord | null>(null);
  const stream = useContainerLogs(containerId, 500, live && active);

  const recordKey = `${active}|${containerId}`;
  const [recordFor, setRecordFor] = useState(recordKey);
  if (recordFor !== recordKey) {
    setRecordFor(recordKey);
    setSelectedRecord(null);
  }

  const records = useMemo<LogDisplayRecord[]>(() => {
    if (live) {
      return stream.records.map((record) => ({
        ...record,
        level: detectLogLevel(record.message),
        text: `${record.timestamp} ${record.message}`,
      }));
    }
    return createSnapshotLogRecords(stream.data);
  }, [live, stream.data, stream.records]);

  const log = useLogFilter(records);
  const filteredRecords = log.records;

  const handleExportLogs = () =>
    downloadLogLines(
      filteredRecords.map((record) => record.text),
      `container-${containerId.slice(0, 12)}.log`,
    );

  if (stream.isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!live && stream.isError) {
    return <ErrorState variant="card" error={stream.error} onRetry={() => stream.refetch()} />;
  }

  const connectionLabel =
    stream.connectionState === 'reconnecting'
      ? t('logs.reconnecting', {
          seconds: Math.ceil(stream.reconnectDelay / 1000),
        })
      : t(`logs.connection.${stream.connectionState}`);

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <LogFilterBar
        {...log.barProps}
        live={live}
        onLiveChange={setLive}
        isRunning={isRunning}
        exportDisabled={filteredRecords.length === 0}
        onExport={handleExportLogs}
      />

      {live ? (
        <output className="block w-fit">
          <Badge variant={stream.connectionState === 'connected' ? 'success' : 'secondary'}>{connectionLabel}</Badge>
        </output>
      ) : null}
      {!isRunning ? <Alert variant="warning" description={t('logs.notRunningWarning')} /> : null}

      <LogViewer
        lines={filteredRecords.map((record) => record.text)}
        levels={filteredRecords.map((record) => record.level)}
        lineIds={filteredRecords.map((record) => record.id)}
        emptyMessage={log.filtering ? t('logs.noMatchingLines', { ns: 'common' }) : t('logs.noLogsAvailable')}
        autoScroll={!log.newestFirst && live}
        onLineSelect={(_, index) => setSelectedRecord(filteredRecords[index] ?? null)}
        getLineAriaLabel={(line, index) => `${line}. ${t('logs.viewLineDetails', { number: index + 1 })}`}
        className="min-h-0 flex-1"
      />

      <LogContextDrawer record={selectedRecord} container={container} onClose={() => setSelectedRecord(null)} />
    </div>
  );
}
