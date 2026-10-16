// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader } from '@resources/components/ui/Card';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { LogViewer } from '@resources/components/LogViewer';
import { LogFilterBar } from '@resources/components/logs/LogFilterBar';
import { createSnapshotLogRecords, downloadLogLines } from '@resources/components/logs/logRecords';
import { useLogFilter } from '@resources/components/logs/useLogFilter';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { useHostLogs } from '../../hooks/useHostLogs';
import type { HostLogSource } from '../../types';

const SOURCES: HostLogSource[] = ['activity', 'system', 'kernel', 'auth', 'docker', 'libvirt', 'agent', 'qemu'];

const KNOWN_NOTICES: readonly string[] = [
  'permission_denied',
  'no_supported_log_source',
  'no_var_log_dir',
  'no_journalctl',
  'journalctl_failed',
  'no_guests',
];

interface HostLogsTabProps {
  envId: string;
}

export function HostLogsTab({ envId }: HostLogsTabProps) {
  const { t } = useTranslation('compute');
  const canReadOsLogs = usePermission(PERM.LOGS_VIEW);
  const [source, setSource] = useState<HostLogSource>('activity');
  const [tail, setTail] = useState(200);
  const [target, setTarget] = useState('');
  const [live, setLive] = useState(false);

  const osDenied = source !== 'activity' && !canReadOsLogs;
  const view = useHostLogs(osDenied ? '' : envId, source, tail, target, live);

  const records = useMemo(() => createSnapshotLogRecords(view.lines.join(String.fromCharCode(10))), [view.lines]);
  const log = useLogFilter(records);
  const lines = useMemo(() => log.records.map((record) => record.text), [log.records]);

  const noticeText = (code: string) =>
    KNOWN_NOTICES.includes(code) ? t(`hosts.logs.notices.${code}`) : t('hosts.logs.notices.unknown', { code });

  return (
    <Card className="flex h-full min-h-0 flex-col">
      <CardHeader className="flex shrink-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="grid gap-2 sm:grid-cols-[200px_110px]">
          <Select
            value={source}
            ariaLabel={t('hosts.logs.sourceLabel')}
            searchable={false}
            options={SOURCES.map((entry) => ({
              value: entry,
              label: t(`hosts.logs.sources.${entry}`),
            }))}
            onChange={(value) => {
              setSource(value as HostLogSource);
              setTarget('');
            }}
          />
          <NumberInput min={1} max={1000} value={tail} aria-label={t('hosts.logs.tailLabel')} onChange={setTail} />
        </div>
        <div className="flex items-center gap-2">
          {source === 'qemu' && (
            <Select
              value={target}
              ariaLabel={t('hosts.logs.guestLabel')}
              placeholder={view.guests.length > 0 ? t('hosts.logs.guestLabel') : t('hosts.logs.noGuests')}
              disabled={view.guests.length === 0}
              options={view.guests.map((guest) => ({
                value: guest.id,
                label: guest.label && guest.label !== guest.id ? `${guest.label} (${guest.id})` : guest.id,
              }))}
              onChange={setTarget}
              className="w-64"
            />
          )}
          {view.isFetching && <Spinner className="size-4" />}
          <Button variant="outline" size="sm" disabled={view.isFetching || osDenied} onClick={() => view.refetch()}>
            <IconRefresh className="size-4" aria-hidden="true" />
            {t('actions.refresh')}
          </Button>
        </div>
      </CardHeader>

      <CardContent className="flex min-h-0 flex-1 flex-col gap-3">
        <LogFilterBar
          {...log.barProps}
          live={live}
          onLiveChange={setLive}
          isRunning={!osDenied}
          exportDisabled={lines.length === 0}
          onExport={() => downloadLogLines(lines, `host-${source}${target ? `-${target}` : ''}-${envId}.log`)}
        />

        {osDenied && (
          <p className="shrink-0 rounded-lg border border-border/70 bg-warning/10 px-3 py-2 text-xs text-foreground">
            {t('hosts.logs.denied')}
          </p>
        )}

        {view.notices.map((notice) => (
          <p
            key={notice}
            className="shrink-0 rounded-lg border border-border/70 bg-warning/10 px-3 py-2 text-xs text-foreground"
          >
            {noticeText(notice)}
          </p>
        ))}

        {view.isError && (
          <div className="flex shrink-0 items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2">
            <div className="min-w-0 text-xs text-foreground">
              <p className="font-medium">{t('hosts.logs.readFailed')}</p>
              {view.error instanceof Error && view.error.message && (
                <p className="mt-0.5 break-words">{view.error.message}</p>
              )}
            </div>
            <StatusError error={view.error instanceof Error ? view.error.message : undefined} />
          </div>
        )}

        {source === 'qemu' && !target && view.guests.length > 0 && (
          <p className="shrink-0 rounded-lg border border-border/70 bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            {t('hosts.logs.pickGuest')}
          </p>
        )}

        <LogViewer
          lines={lines}
          levels={log.records.map((record) => record.level)}
          lineIds={log.records.map((record) => record.id)}
          colorizeSeverity
          autoScroll={!log.newestFirst}
          emptyMessage={log.filtering ? t('logs.noMatchingLines', { ns: 'common' }) : t('hosts.logs.empty')}
          className="min-h-[240px] flex-1"
        />
      </CardContent>
    </Card>
  );
}
