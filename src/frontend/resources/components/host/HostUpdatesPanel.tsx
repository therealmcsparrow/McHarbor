// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { IconDownload, IconRefresh } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { LogViewer } from '@resources/components/LogViewer';
import {
  useHostOSInfo,
  useHostUpdateCheck,
  useHostUpdateProgress,
  useHostUpdateState,
  type HostTarget,
} from '@resources/hooks/useHostUpdates';
import { HostOSInfoCard } from './HostOSInfoCard';
import { HostRebootCard } from './HostRebootCard';
import { HostUpdateApplyDialog } from './HostUpdateApplyDialog';
import { HostUpdateScheduleCard } from './HostUpdateScheduleCard';
import { hostPermission } from '@resources/hooks/useHostUpdates';

type HostUpdatesPanelProps = {
  target: HostTarget;
};

export function HostUpdatesPanel({ target }: HostUpdatesPanelProps) {
  const { t } = useTranslation('environments');
  const formatPrefs = useLocaleFormat();
  const [startedJobId, setJobId] = useState<string | null>(null);
  const [applyOpen, setApplyOpen] = useState(false);

  const osInfo = useHostOSInfo(target, true);
  const state = useHostUpdateState(target, true);
  const check = useHostUpdateCheck(target);
  const jobId = startedJobId ?? state.data?.runningJobId ?? null;
  const { progress } = useHostUpdateProgress(target, jobId);

  const updates = state.data?.updates ?? [];
  const busy = progress.running;
  // The host could not be read at all — an agent that is offline, too old, or
  // containerised with no view of its host. Nothing below can succeed, so every
  // control goes dead rather than offering an action that will only 500.
  const unreadable = Boolean(osInfo.data?.unsupported) || osInfo.isError;

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <HostOSInfoCard info={osInfo.data} loading={osInfo.isPending} error={osInfo.isError} />

      <div className="mc-glass rounded-lg p-6">
        <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="text-sm font-medium text-foreground">{t('detail.hostUpdates.title')}</h3>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {state.data?.lastCheckedAt
                ? t('detail.hostUpdates.lastChecked', {
                    when: formatDate(state.data.lastCheckedAt, formatPrefs),
                  })
                : t('detail.hostUpdates.neverChecked')}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              permission={hostPermission(target, 'updatesView')}
              disabled={unreadable || check.isPending || busy}
              onClick={() => check.mutate()}
            >
              {check.isPending ? (
                <Spinner className="mr-1.5 h-4 w-4" />
              ) : (
                <IconRefresh className="mr-1.5 h-4 w-4" aria-hidden="true" />
              )}
              {t('detail.hostUpdates.checkNow')}
            </Button>
            <Button
              size="sm"
              permission={hostPermission(target, 'updatesManage')}
              disabled={unreadable || busy || !state.data?.available}
              onClick={() => setApplyOpen(true)}
            >
              {busy ? (
                <Spinner className="mr-1.5 h-4 w-4" />
              ) : (
                <IconDownload className="mr-1.5 h-4 w-4" aria-hidden="true" />
              )}
              {t('detail.hostUpdates.updateNow')}
            </Button>
          </div>
        </div>

        {check.isError ? (
          <ErrorState variant="inline" title={t('detail.hostUpdates.checkFailed')} error={check.error} />
        ) : null}

        {unreadable ? (
          <p className="py-2 text-sm text-muted-foreground">{t('detail.hostUpdates.unavailable')}</p>
        ) : state.isPending ? (
          <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
            <Spinner className="h-4 w-4" />
            {t('detail.hostUpdates.loading')}
          </div>
        ) : state.data?.available ? (
          <div className="space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="warning">{t('detail.hostUpdates.pending', { count: state.data.count })}</Badge>
              {state.data.manager ? <Badge variant="outline">{state.data.manager}</Badge> : null}
            </div>
            <ul className="max-h-64 space-y-0.5 overflow-auto rounded-md border border-border p-3 font-mono text-xs text-muted-foreground">
              {updates.map((entry) => (
                <li key={entry} className="break-all">
                  {entry}
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <p className="py-2 text-sm text-muted-foreground">{t('detail.hostUpdates.upToDate')}</p>
        )}
      </div>

      <HostRebootCard
        target={target}
        rebootRequired={Boolean(osInfo.data?.rebootRequired)}
        disabled={unreadable || busy}
      />

      {jobId ? (
        <div className="mc-glass flex min-h-0 flex-1 flex-col rounded-lg p-6">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-medium text-foreground">{t('detail.hostUpdates.progressTitle')}</h3>
            {progress.running ? (
              <Badge variant="outline">
                <Spinner className="mr-1.5 h-3 w-3" />
                {t('detail.hostUpdates.running', {
                  count: progress.lines.length,
                })}
              </Badge>
            ) : progress.done && progress.success ? (
              <Badge variant="success">{t('detail.hostUpdates.finished')}</Badge>
            ) : progress.done ? (
              <Badge variant="destructive">{t('detail.hostUpdates.failed')}</Badge>
            ) : null}
          </div>

          {progress.error ? <Alert variant="destructive" className="mb-3" description={progress.error} /> : null}
          {progress.hint === 'skipUnavailable' ? (
            <Alert variant="warning" className="mb-3" description={t('detail.hostUpdates.hintSkipUnavailable')} />
          ) : null}

          <LogViewer
            lines={progress.lines}
            emptyMessage={t('detail.hostUpdates.progressWaiting')}
            autoScroll
            className="min-h-[220px] flex-1"
          />
        </div>
      ) : null}

      <HostUpdateScheduleCard
        target={target}
        schedule={state.data?.schedule}
        disabled={unreadable}
        recoveryUnsupported={state.data?.recoveryUnsupported}
      />

      <HostUpdateApplyDialog
        target={target}
        open={applyOpen}
        count={state.data?.count ?? 0}
        defaults={state.data?.schedule}
        recoveryUnsupported={state.data?.recoveryUnsupported}
        onOpenChange={setApplyOpen}
        onStarted={setJobId}
      />
    </div>
  );
}
