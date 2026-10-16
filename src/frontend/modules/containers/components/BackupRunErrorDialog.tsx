// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
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
import { Box, Flex } from '@resources/components/ui/Layout';
import { Text as BodyText } from '@resources/components/ui/Typography';
import type { ContainerBackupRun } from '@resources/hooks/useContainerBackups';

// BackupRunErrorDialog shows every error attached to a failed run: the run's own
// failure and any per-destination upload failure. A run can succeed at the
// archive step and still fail to reach two of its three destinations, so the
// per-destination errors are not a detail — they are often the whole story.
export function BackupRunErrorDialog({ run, onClose }: { run: ContainerBackupRun | null; onClose: () => void }) {
  const { t } = useTranslation('containers');

  const failedDestinations = (run?.destinations ?? []).filter((destination) => destination.status === 'failure');
  const runError = run?.error?.trim();

  return (
    <Dialog
      open={!!run}
      onOpenChange={(isOpen) => {
        if (!isOpen) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('backups.errorDialog.title')}</DialogTitle>
          <DialogDescription>{t('backups.errorDialog.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {runError && (
            <Box className="rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <Flex align="center" gap={2} className="mb-1">
                <IconAlertTriangle className="size-4 shrink-0 text-destructive" aria-hidden="true" />
                <BodyText size="sm" weight="medium" tone="destructive">
                  {t('backups.errorDialog.runError')}
                </BodyText>
              </Flex>
              <BodyText size="sm" className="break-words whitespace-pre-wrap">
                {runError}
              </BodyText>
            </Box>
          )}
          {failedDestinations.map((destination) => (
            <Box key={destination.id} className="rounded-md border border-border p-3">
              <Flex align="center" gap={2} className="mb-1">
                <IconAlertTriangle className="size-4 shrink-0 text-destructive" aria-hidden="true" />
                <BodyText size="sm" weight="medium">
                  {destination.storageLocationName || t('backups.errorDialog.unknownDestination')}
                </BodyText>
              </Flex>
              <BodyText size="sm" className="break-words whitespace-pre-wrap">
                {destination.error?.trim() || t('backups.destinationFailure.unknownError')}
              </BodyText>
            </Box>
          ))}
          {!runError && failedDestinations.length === 0 && (
            <BodyText size="sm" tone="muted">
              {t('backups.errorDialog.empty')}
            </BodyText>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('common.close', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
