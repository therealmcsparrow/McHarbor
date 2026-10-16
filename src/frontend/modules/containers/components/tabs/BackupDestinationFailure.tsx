// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconRefresh } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { type ContainerBackupRunDestination } from '@resources/hooks/useContainerBackups';
import { destinationLabel, DestinationIcon } from './BackupDestinationItems';

export function FailureDestinationButton({
  destination,
  onRetryUpload,
  retryPending,
}: {
  destination: ContainerBackupRunDestination;
  onRetryUpload: (destination: ContainerBackupRunDestination) => void;
  retryPending: boolean;
}) {
  const { t } = useTranslation('containers');
  const [errorOpen, setErrorOpen] = useState(false);
  const destinationName = destinationLabel(destination, t('backups.storage.external'));

  return (
    <Dialog open={errorOpen} onOpenChange={setErrorOpen}>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={`${t('backups.storageStatus.failure')}: ${destinationName}`}
            data-testid={`destination-failure-${destination.id}`}
            onClick={() => setErrorOpen(true)}
            className="size-7 hover:bg-destructive/10"
          >
            <DestinationIcon destination={destination} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{destinationName}</TooltipContent>
      </Tooltip>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <div className="flex items-center gap-2">
            <IconAlertTriangle className="size-5 text-destructive" aria-hidden="true" />
            <DialogTitle>{t('backups.destinationFailure.title')}</DialogTitle>
          </div>
          <DialogDescription>{destinationName}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
            {destination.error || t('backups.destinationFailure.unknownError')}
          </div>
          <p className="text-xs text-muted-foreground">{t('backups.destinationFailure.retryHint')}</p>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => setErrorOpen(false)} data-testid="destination-failure-close">
            {t('backups.destinationFailure.close')}
          </Button>
          <Button
            onClick={() => {
              setErrorOpen(false);
              onRetryUpload(destination);
            }}
            disabled={retryPending}
            data-testid="destination-failure-retry"
          >
            <IconRefresh className="size-4" aria-hidden="true" />
            {t('backups.destinationFailure.retry')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
