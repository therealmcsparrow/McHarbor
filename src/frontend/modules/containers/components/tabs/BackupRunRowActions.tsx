// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconPlayerStop, IconRestore, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { PERM } from '@core/auth/permissionKeys';
import { type ContainerBackupRun, containerBackupDownloadUrl } from '@resources/hooks/useContainerBackups';

function formatBytes(value: number) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

type BackupRunRowActionsProps = {
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onCancel: (run: ContainerBackupRun) => void;
  deletePending: boolean;
  cancelPending: boolean;
};

// The right-hand strip of one row in BackupRunsPanel: archive size plus the
// download / cancel / delete controls the run's state allows.
export function BackupRunRowActions({
  run,
  onRestore,
  onDelete,
  onCancel,
  deletePending,
  cancelPending,
}: BackupRunRowActionsProps) {
  const { t } = useTranslation('containers');
  // A run with a stored archive restores straight from it. When a destination
  // already offers Restore, a second button here would duplicate it.
  const restoreFromArchive =
    run.operation === 'backup' &&
    run.status === 'success' &&
    !!run.archivePath &&
    !(run.destinations ?? []).some((destination) => destination.status === 'success');

  return (
    <div className="flex shrink-0 items-center gap-2">
      <span className="text-xs text-muted-foreground">
        {/* A restore run writes into the container and keeps no archive. */}
        {run.operation === 'restore' ? '—' : formatBytes(run.archiveSize)}
      </span>
      {restoreFromArchive && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('backups.runs.table.restore')}
          permission={PERM.RESTORE_RUN}
          onClick={() => onRestore(run)}
          data-testid={`run-restore-${run.id}`}
        >
          <IconRestore className="size-4" aria-hidden="true" />
        </Button>
      )}
      {run.operation === 'backup' && run.status === 'success' && run.archivePath && (
        <Button
          variant="ghost"
          size="icon-sm"
          asChild
          aria-label={t('backups.download')}
          permission={PERM.BACKUPS_DOWNLOAD}
        >
          <a href={containerBackupDownloadUrl(run.id)} download>
            <IconDownload className="size-4" />
          </a>
        </Button>
      )}
      {run.status === 'running' && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('backups.cancelRun')}
              permission={PERM.BACKUPS_MANAGE}
              data-testid={`cancel-run-${run.id}`}
              onClick={() => onCancel(run)}
              disabled={cancelPending}
              className="text-warning hover:border-warning/30 hover:bg-warning/10"
            >
              <IconPlayerStop className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('backups.cancelRun')}</TooltipContent>
        </Tooltip>
      )}
      {run.status !== 'running' && (
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('backups.deleteRun')}
          permission={PERM.BACKUPS_DELETE}
          onClick={() => onDelete(run)}
          disabled={deletePending}
        >
          <IconTrash className="size-4" />
        </Button>
      )}
    </div>
  );
}
