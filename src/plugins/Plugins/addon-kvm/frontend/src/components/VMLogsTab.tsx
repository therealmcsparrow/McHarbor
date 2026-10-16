// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { IconRefresh } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import { LogFilterBar } from '@resources/components/logs/LogFilterBar';
import { createSnapshotLogRecords, downloadLogLines } from '@resources/components/logs/logRecords';
import { useLogFilter } from '@resources/components/logs/useLogFilter';
import type { GuestLog, VM } from '../types';

type LogSource = 'vm' | 'tpm';

const TAILS = [200, 500, 1000, 2000];
const LIVE_POLL_MS = 3000;

export function VMLogsTab({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const [source, setSource] = useState<LogSource>('vm');
  const [tail, setTail] = useState(200);
  const [live, setLive] = useState(false);

  const isRunning = vm.state === 'running';

  const { data, isPending, isError, error, isFetching, refetch } = useQuery({
    queryKey: ['vms', vm.id, 'logs', source, tail],
    queryFn: async () =>
      assertSuccess(
        await api.get<GuestLog>(`/vms/${vm.id}/logs`, {
          source,
          tail: String(tail),
        }),
      ),
    retry: false,
    refetchInterval: live && isRunning ? LIVE_POLL_MS : false,
  });

  const records = useMemo(() => createSnapshotLogRecords((data?.lines ?? []).join('\n')), [data?.lines]);

  const log = useLogFilter(records);
  const filteredRecords = log.records;

  const handleExport = () =>
    downloadLogLines(
      filteredRecords.map((record) => record.text),
      t('logs.exportName', { name: vm.name, source }),
    );

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="w-52">
          <Select
            value={source}
            ariaLabel={t('logs.source')}
            options={[
              { value: 'vm', label: t('logs.sources.vm') },
              { value: 'tpm', label: t('logs.sources.tpm') },
            ]}
            onChange={(next) => setSource(next as LogSource)}
          />
        </div>
        <div className="w-36">
          <Select
            value={String(tail)}
            ariaLabel={t('logs.tail')}
            options={TAILS.map((value) => ({
              value: String(value),
              label: t('logs.lines', { count: value }),
            }))}
            onChange={(next) => setTail(Number(next))}
          />
        </div>
        <Button size="sm" variant="outline" disabled={isFetching} onClick={() => void refetch()}>
          {isFetching ? (
            <Spinner className="mr-1.5 h-4 w-4" />
          ) : (
            <IconRefresh className="mr-1.5 h-4 w-4" aria-hidden="true" />
          )}
          {t('actions.refresh')}
        </Button>
      </div>

      <LogFilterBar
        {...log.barProps}
        live={live}
        onLiveChange={setLive}
        isRunning={isRunning}
        exportDisabled={filteredRecords.length === 0}
        onExport={handleExport}
      />

      {live ? (
        <output aria-live="polite" className="block w-fit">
          <Alert variant="info" description={t('logs.liveHint')} />
        </output>
      ) : null}

      {isError ? (
        <Alert
          variant="destructive"
          title={t('logs.failed')}
          description={error instanceof Error ? error.message : t('logs.failed')}
        />
      ) : null}

      {data?.notices?.length ? (
        <Alert
          variant="warning"
          title={t('logs.partial')}
          description={data.notices.map((notice) => t(`logs.notices.${notice}`)).join(' ')}
        />
      ) : null}

      {isPending ? (
        <div className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
          <Spinner className="h-4 w-4" />
          {t('logs.loading')}
        </div>
      ) : (
        <LogViewer
          lines={filteredRecords.map((record) => record.text)}
          levels={filteredRecords.map((record) => record.level)}
          lineIds={filteredRecords.map((record) => record.id)}
          emptyMessage={log.filtering ? t('logs.noMatchingLines', { ns: 'common' }) : t('logs.empty')}
          autoScroll={!log.newestFirst && live}
          className="min-h-0 flex-1"
        />
      )}
    </div>
  );
}
