// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconFileText, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { LogFilterBar } from '@resources/components/logs/LogFilterBar';
import { createSnapshotLogRecords, downloadLogLines } from '@resources/components/logs/logRecords';
import { useLogFilter } from '@resources/components/logs/useLogFilter';
import { api } from '@core/api/client';
import { hostLogNoticeText } from './hostLogNotices';
import type { HostLogResult, HostLogSource } from './hostLogNotices';
import { assertSuccess } from '@resources/utils/api-mutation';

type LogSource = HostLogSource;

type EnvironmentHostLogsTabProps = {
  envId: string;
};

// A host OS log read is a shell call on the environment, so the tab polls only while
// Live is on; otherwise it refreshes on demand and when the source or tail changes.
export function EnvironmentHostLogsTab({ envId }: EnvironmentHostLogsTabProps) {
  const { t } = useTranslation('environments');
  const [source, setSource] = useState<LogSource>('system');
  const [tail, setTail] = useState(200);
  const [live, setLive] = useState(false);
  const enabled = !!envId;
  const { data, isLoading, isFetching, isError, error, refetch } = useQuery({
    queryKey: ['environment-host-logs', envId, source, tail],
    queryFn: () =>
      api
        .get<HostLogResult>('/system/os-logs', {
          env: envId,
          source,
          tail: String(tail),
        })
        .then(assertSuccess),
    enabled,
    refetchOnWindowFocus: false,
    staleTime: 30_000,
    refetchInterval: live ? 5_000 : false,
    placeholderData: (previous) => previous,
  });

  const records = useMemo(
    () => createSnapshotLogRecords((data?.lines ?? []).join(String.fromCharCode(10))),
    [data?.lines],
  );
  const log = useLogFilter(records);
  const lines = useMemo(() => log.records.map((record) => record.text), [log.records]);

  useEffect(() => {
    if (!enabled) return;
    void refetch();
  }, [enabled, envId, refetch]);

  const sourceOptions = useMemo(
    () => [
      { value: 'system', label: t('detail.hostLogs.sources.system') },
      { value: 'kernel', label: t('detail.hostLogs.sources.kernel') },
      { value: 'auth', label: t('detail.hostLogs.sources.auth') },
      { value: 'docker', label: t('detail.hostLogs.sources.docker') },
    ],
    [t],
  );

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconFileText className="size-5 text-muted-foreground" />
            {t('detail.hostLogs.title')}
          </CardTitle>
          <CardDescription>{t('detail.hostLogs.description')}</CardDescription>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={source}
            onChange={(value) => setSource(value as LogSource)}
            options={sourceOptions}
            searchable={false}
            ariaLabel={t('detail.hostLogs.sourceLabel')}
            className="w-44"
          />
          <NumberInput
            size="sm"
            min={1}
            max={1000}
            value={tail}
            aria-label={t('detail.hostLogs.tailLabel')}
            onChange={setTail}
            className="w-36"
          />
          <Button variant="outline" onClick={() => void refetch()} disabled={isFetching}>
            <IconRefresh className="size-4" />
            {t('detail.hostLogs.refresh')}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {enabled && (
          <div className="mb-3">
            <LogFilterBar
              {...log.barProps}
              live={live}
              onLiveChange={setLive}
              exportDisabled={lines.length === 0}
              onExport={() => downloadLogLines(lines, `host-${source}-${envId}.log`)}
            />
          </div>
        )}
        {!enabled && (
          <div className="rounded-lg border border-border bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
            {t('detail.hostLogs.unavailable')}
          </div>
        )}
        {enabled && data?.notices && data.notices.length > 0 && (
          <div className="mb-3 space-y-2">
            {data.notices.map((notice) => (
              <div
                key={notice}
                className="rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning"
              >
                {hostLogNoticeText(t, notice)}
              </div>
            ))}
          </div>
        )}
        {enabled && data?.fallbackFile && (
          <div className="mb-3 rounded-lg border border-border bg-muted/20 px-3 py-2 text-sm text-muted-foreground">
            {t('detail.hostLogs.fallbackNote', { file: data.fallbackFile })}
          </div>
        )}
        {enabled && data?.availableFiles && data.availableFiles.length > 0 && (
          <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
            <span className="font-medium">{t('detail.hostLogs.availableFilesLabel')}</span>
            {data.availableFiles.map((file) => (
              <code key={file} className="rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">
                {file}
              </code>
            ))}
          </div>
        )}
        {enabled && isLoading && (
          <div className="flex h-64 items-center justify-center">
            <Spinner />
          </div>
        )}
        {enabled && isError && !isLoading && (
          <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-muted/20 text-sm text-muted-foreground">
            {[t('detail.hostLogs.unavailable'), error?.message].filter(Boolean).join(' — ')}
          </div>
        )}
        {enabled && !isLoading && !isError && (
          <LogViewer
            colorizeSeverity
            lines={lines}
            levels={log.records.map((record) => record.level)}
            lineIds={log.records.map((record) => record.id)}
            autoScroll={!log.newestFirst}
            emptyMessage={log.filtering ? t('logs.noMatchingLines', { ns: 'common' }) : t('detail.hostLogs.empty')}
            className="h-[480px]"
          />
        )}
      </CardContent>
    </Card>
  );
}
