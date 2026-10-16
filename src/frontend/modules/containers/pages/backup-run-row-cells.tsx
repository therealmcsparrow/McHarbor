// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The two widest cells of the backup runs table: the per-destination
// status strip and the row action buttons. Split out of
// backup-run-cells.tsx; markup and `data-testid` hooks are unchanged.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconExternalLink, IconRestore, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { PERM } from '@core/auth/permissionKeys';
import { cn } from '@resources/utils/cn';
import { type StorageLocationType } from '@resources/hooks/useStorageLocations';
import { storageProvider } from '@resources/components/storage-location-options';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';
import type { StorageAdapterSpec } from '@resources/hooks/useStorageAdapters';
import { DASH, destinationLabelFor } from './backup-run-cells';

function destinationLocationType(locationType: string | undefined): StorageLocationType {
  return (locationType || 'local') as StorageLocationType;
}

function destinationTint(destinationStatus: string): string {
  if (destinationStatus === 'success') return 'text-success';
  if (destinationStatus === 'failure') return 'text-destructive';
  return 'text-muted-foreground';
}

export function RunDestinationsCell({
  run,
  pluginAdapters,
}: {
  run: ContainerBackupRun;
  pluginAdapters: Record<string, StorageAdapterSpec> | null | undefined;
}) {
  const { t } = useTranslation('containers');
  const destinations = run.destinations ?? [];
  const hasSuccess = destinations.some((d) => d.status === 'success');
  const hasFailure = destinations.some((d) => d.status === 'failure');
  const hasUploading = destinations.some((d) => d.status === 'uploading');
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {destinations.length === 0 ? (
        <span className="text-xs text-muted-foreground">{DASH}</span>
      ) : (
        destinations.map((destination) => {
          const destinationName = destination.storageLocationName || destinationLabelFor(destination);
          const provider = storageProvider(destinationLocationType(destination.locationType), pluginAdapters);
          const tint = destinationTint(destination.status);
          return (
            <Tooltip key={destination.id}>
              <TooltipTrigger asChild>
                <span
                  className={cn(
                    'inline-flex size-7 cursor-default items-center justify-center rounded-md border border-border bg-background hover:bg-muted/50',
                    tint,
                  )}
                  aria-label={`${destination.status}: ${destinationName}`}
                >
                  {provider.icon}
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <div className="space-y-0.5">
                  <div className="font-medium">{destinationName}</div>
                  <div className="text-[10px] text-muted-foreground">
                    {t(`backups.storageStatus.${destination.status}`)}
                  </div>
                </div>
              </TooltipContent>
            </Tooltip>
          );
        })
      )}
      {hasUploading && (
        <span className="ml-1 text-[10px] text-muted-foreground">
          {hasSuccess ? '' : t('backups.runs.table.runningHint')}
        </span>
      )}
      {hasFailure && !hasSuccess && (
        <span className="ml-1 text-[10px] text-destructive">{t('backups.runs.table.failedHint')}</span>
      )}
    </div>
  );
}

export function RunActionsCell({
  run,
  onRestore,
  onDownload,
  onDelete,
  onOpenContainer,
}: {
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDownload: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onOpenContainer: (environmentId: string, containerId: string) => void;
}) {
  const { t } = useTranslation('containers');
  const canRestore = run.operation === 'backup' && run.status === 'success';
  const canDownload = run.operation === 'backup' && run.status === 'success' && !!run.archivePath;
  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('backups.runs.table.restore')}
        permission={PERM.RESTORE_RUN}
        onClick={() => onRestore(run)}
        disabled={!canRestore}
        data-testid={`run-restore-${run.id}`}
      >
        <IconRestore className="size-4" aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('backups.runs.table.download')}
        permission={PERM.BACKUPS_DOWNLOAD}
        disabled={!canDownload}
        onClick={() => onDownload(run)}
        data-testid={`run-download-${run.id}`}
      >
        <IconDownload className="size-4" aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('backups.runs.table.delete')}
        permission={PERM.BACKUPS_DELETE}
        onClick={() => onDelete(run)}
        data-testid={`run-delete-${run.id}`}
      >
        <IconTrash className="size-4 text-destructive" aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label={t('backups.runs.table.open')}
        onClick={() => onOpenContainer(run.environmentId, run.containerId)}
      >
        <IconExternalLink className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
