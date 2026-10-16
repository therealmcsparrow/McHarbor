// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { useTranslation } from 'react-i18next';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import { type ContainerBackupRun, type ContainerBackupRunDestination } from '@resources/hooks/useContainerBackups';
import { destinationLabel, DestinationIcon, SuccessDestinationButton } from './BackupDestinationItems';
import { FailureDestinationButton } from './BackupDestinationFailure';

type BackupRunDestinationsProps = {
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  onRetryUpload: (destination: ContainerBackupRunDestination) => void;
  retryPendingFor?: string;
};

export function BackupRunDestinations({
  run,
  onRestore,
  onDelete,
  onRetryUpload,
  retryPendingFor,
}: BackupRunDestinationsProps) {
  const { t } = useTranslation('containers');
  const destinations = run.destinations ?? [];
  if (destinations.length === 0) return null;
  // Only completed backup runs with a downloadable archive expose the
  // Download menu item. Restore runs reuse the same destination row
  // pattern but have no archive to fetch.
  const canDownload = run.operation === 'backup' && run.status === 'success' && !!run.archivePath;

  return (
    <div className={cn('mt-2 flex flex-wrap items-center gap-2')} data-testid={`destinations-${run.id}`}>
      {destinations.map((destination) => {
        if (destination.status === 'success') {
          return (
            <SuccessDestinationButton
              key={destination.id}
              destination={destination}
              run={run}
              onRestore={onRestore}
              onDelete={onDelete}
              canDownload={canDownload}
            />
          );
        }
        if (destination.status === 'failure') {
          return (
            <FailureDestinationButton
              key={destination.id}
              destination={destination}
              onRetryUpload={onRetryUpload}
              retryPending={retryPendingFor === destination.id}
            />
          );
        }
        // uploading — render the icon + a compact progress bar so
        // the operator sees byte progress next to the destination
        // while the upload goroutine runs. Bytes flow in via the
        // regular runs-query polling loop.
        return (
          <UploadingDestinationItem
            key={destination.id}
            destination={destination}
            label={t('backups.destinationFailure.retryUploading')}
          />
        );
      })}
    </div>
  );
}

// UploadingDestinationItem shows the storage icon + a compact
// progress bar that tracks bytes_uploaded / bytes_total. The bytes
// are populated by the existing per-destination progress callback
// (storage_upload.go) and surface in the runs query, which the
// parent already polls. Renders the storage-type icon (so the
// operator can see which destination is uploading) plus a width
// that adjusts to the available space, with both byte counts and
// percentage visible.
function UploadingDestinationItem({
  destination,
  label,
}: {
  destination: ContainerBackupRunDestination;
  label: string;
}) {
  const { t } = useTranslation('containers');
  const destinationName = destinationLabel(destination, t('backups.storage.external'));
  const uploaded = destination.bytesUploaded ?? 0;
  const total = destination.bytesTotal ?? 0;
  const percent = total > 0 ? Math.min(100, Math.round((uploaded / total) * 100)) : 0;
  const byteText =
    total > 0 ? `${formatProgressBytes(uploaded)} / ${formatProgressBytes(total)}` : formatProgressBytes(uploaded);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          className="inline-flex max-w-[260px] min-w-[180px] flex-1 items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1"
          data-testid={`destination-uploading-${destination.id}`}
          aria-label={`${label}: ${destinationName}`}
        >
          <DestinationIcon destination={destination} />
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <div className="flex items-center justify-between gap-2 text-[11px] leading-none text-muted-foreground">
              <span className="truncate font-medium text-foreground">{destinationName}</span>
              <span className="font-mono tabular-nums">{percent}%</span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-muted">
              <div
                className="h-full w-(--w) bg-primary transition-[width] duration-300 ease-out"
                style={cssVars({ w: `${percent}%` })}
              />
            </div>
            <span className="font-mono text-[10px] text-muted-foreground tabular-nums">{byteText}</span>
          </div>
        </div>
      </TooltipTrigger>
      <TooltipContent>{destinationName}</TooltipContent>
    </Tooltip>
  );
}

function formatProgressBytes(value: number) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}
