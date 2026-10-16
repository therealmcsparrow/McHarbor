// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck } from '@tabler/icons-react';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDate } from '@resources/utils/format';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';

// Human-readable mapping for restore_* progress stages emitted by
// the backend. The container_backup service writes these to the
// run record throughout the restore pipeline.
const RESTORE_STAGE_LABELS: Record<string, string> = {
  restore_connecting: 'backups.progress.restore_connecting',
  restore_inspecting: 'backups.progress.restore_inspecting',
  restore_scanning: 'backups.progress.restore_scanning',
  restore_image: 'backups.progress.restore_image',
  restore_filesystem: 'backups.progress.restore_filesystem',
  restore_mounts: 'backups.progress.restore_mounts',
};

function describeRestoreStage(
  stage: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (!stage) return '';
  const key = RESTORE_STAGE_LABELS[stage];
  if (key) {
    return t(key);
  }
  return t(`backups.progress.${stage}`);
}

// RestoreProgressView is the live-progress body of the dialog
// shown after the operator has kicked off a restore. Renders the
// status icon, the human-readable stage / message, and the
// terminal-state copy (success / failure).
export function RestoreProgressView({
  run,
  loading,
  startedAt,
}: {
  run?: ContainerBackupRun;
  loading: boolean;
  startedAt: string;
}) {
  const { t } = useTranslation('containers');
  const formatPrefs = useLocaleFormat();
  const isRunning = run?.status === 'running';
  const isSuccess = run?.status === 'success';
  const isFailure = run?.status === 'failure';
  const stageLabel = describeRestoreStage(run?.progressStage, t);
  const message = run?.progressMessage ?? '';
  const startedLabel = useMemo(() => {
    try {
      const formatted = formatDate(startedAt, formatPrefs);
      return formatted === '-' ? startedAt : formatted;
    } catch {
      return startedAt;
    }
  }, [startedAt, formatPrefs]);

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3 rounded-lg border border-border bg-background p-4">
        {isSuccess ? (
          <IconCheck className="mt-0.5 size-5 shrink-0 text-success" aria-hidden="true" />
        ) : isFailure ? (
          <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" aria-hidden="true" />
        ) : (
          <Spinner size="md" className="mt-0.5 shrink-0" />
        )}
        <div className="min-w-0 flex-1 space-y-1">
          <p className="text-sm font-medium text-foreground">
            {isSuccess
              ? t('backups.restoreCompleteHeading')
              : isFailure
                ? t('backups.restoreFailedHeading')
                : t('backups.restoringHeading')}
          </p>
          <p className="text-xs text-muted-foreground">{t('backups.startedAt', { value: startedLabel })}</p>
          {isRunning && (
            <div className="space-y-1 pt-1">
              {stageLabel && <p className="text-xs font-medium text-foreground">{stageLabel}</p>}
              {message && message !== stageLabel && (
                <p className="text-xs break-words text-muted-foreground">{message}</p>
              )}
            </div>
          )}
          {isFailure && run?.error && <p className="text-xs break-words text-destructive">{run.error}</p>}
          {isSuccess && <p className="text-xs text-success">{t('backups.restoreCompleteHint')}</p>}
          {loading && !run && <p className="text-xs text-muted-foreground">{t('backups.restoreChoicesLoading')}</p>}
        </div>
      </div>
    </div>
  );
}
