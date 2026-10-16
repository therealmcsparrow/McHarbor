// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCircleCheckFilled, IconCircleXFilled, IconDownload, IconHistory, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import {
  containerBackupDownloadUrl,
  type ContainerBackupRun,
  type ContainerBackupRunDestination,
} from '@resources/hooks/useContainerBackups';
import { type StorageLocationType } from '@resources/hooks/useStorageLocations';
import { useStorageAdapters } from '@resources/hooks/useStorageAdapters';
import { storageProvider } from '@resources/components/storage-location-options';

const FALLBACK_LOCATION_TYPE = 'storage';

export function destinationLabel(destination: ContainerBackupRunDestination, fallback: string) {
  if (destination.storageLocationName) return destination.storageLocationName;
  if (destination.storageLocationId) return destination.storageLocationId;
  return fallback;
}

// DestinationIcon renders the storage-type glyph as the primary icon
// (folder for local, OneDrive cloud, SharePoint office, AWS, Google
// Drive, etc.) with a small status badge in the corner so the
// success / failure / uploading signal is visible at a glance. The
// badge uses a card-colored background so it visually detaches from
// the underlying icon. For uploading destinations the badge is a
// spinning ring so the operator can still see the storage type
// underneath.
export function DestinationIcon({ destination }: { destination: ContainerBackupRunDestination }) {
  const { t } = useTranslation('containers');
  const { data: pluginAdapters } = useStorageAdapters();
  const provider = storageProvider((destination.locationType || 'local') as StorageLocationType, pluginAdapters);
  const tintClass =
    destination.status === 'success'
      ? 'text-success'
      : destination.status === 'uploading'
        ? 'text-primary'
        : 'text-destructive';
  return (
    <span
      className="relative inline-flex items-center justify-center"
      aria-label={`${destination.locationType || FALLBACK_LOCATION_TYPE} ${destination.status}`}
    >
      <span className={cn('inline-flex size-5 items-center justify-center', tintClass)}>{provider.icon}</span>
      <span className={cn('absolute -right-0.5 -bottom-0.5 inline-flex size-4 items-center justify-center bg-card')}>
        {destination.status === 'success' ? (
          <IconCircleCheckFilled className="size-3 text-success" aria-label={t('backups.storageStatus.success')} />
        ) : destination.status === 'failure' ? (
          <IconCircleXFilled className="size-3 text-destructive" aria-label={t('backups.storageStatus.failure')} />
        ) : (
          <Spinner size="sm" className="text-primary" aria-label={t('backups.storageStatus.uploading')} />
        )}
      </span>
    </span>
  );
}

export function SuccessDestinationButton({
  destination,
  run,
  onRestore,
  onDelete,
  canDownload,
}: {
  destination: ContainerBackupRunDestination;
  run: ContainerBackupRun;
  onRestore: (run: ContainerBackupRun) => void;
  onDelete: (run: ContainerBackupRun) => void;
  canDownload: boolean;
}) {
  const { t } = useTranslation('containers');
  const destinationName = destinationLabel(destination, t('backups.storage.external'));
  const downloadUrl = canDownload ? containerBackupDownloadUrl(run.id) : null;

  return (
    <DropdownMenu.Root>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenu.Trigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={`${t('backups.storageStatus.success')}: ${destinationName}`}
              data-testid={`destination-success-${destination.id}`}
              className="size-7 hover:bg-success/10"
            >
              <DestinationIcon destination={destination} />
            </Button>
          </DropdownMenu.Trigger>
        </TooltipTrigger>
        <TooltipContent>{destinationName}</TooltipContent>
      </Tooltip>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={4}
          className="mc-glass-overlay mc-menu z-50 min-w-[12rem] overflow-hidden rounded-lg p-1 text-popover-foreground"
        >
          <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">{destinationName}</div>
          <DropdownMenu.Separator className="my-1 h-px bg-border" />
          {downloadUrl && (
            <DropdownMenu.Item
              className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors outline-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
              onSelect={() => {
                // Trigger the download by creating a transient anchor.
                // Using onSelect keeps the menu dismissable in a single
                // interaction instead of leaving it open after a click.
                const link = document.createElement('a');
                link.href = downloadUrl;
                link.download = '';
                link.rel = 'noopener';
                document.body.appendChild(link);
                link.click();
                link.remove();
              }}
            >
              <IconDownload className="size-4" aria-hidden="true" />
              {t('backups.destinationMenu.download')}
            </DropdownMenu.Item>
          )}
          <DropdownMenu.Item
            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors outline-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            onSelect={() => onRestore(run)}
          >
            <IconHistory className="size-4" aria-hidden="true" />
            {t('backups.destinationMenu.restore')}
          </DropdownMenu.Item>
          <DropdownMenu.Item
            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-destructive transition-colors outline-none focus:bg-destructive/10 data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
            onSelect={() => onDelete(run)}
          >
            <IconTrash className="size-4" aria-hidden="true" />
            {t('backups.destinationMenu.delete')}
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}
