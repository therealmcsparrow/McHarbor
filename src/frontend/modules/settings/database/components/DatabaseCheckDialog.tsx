// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@resources/components/ui/Dialog';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { ACTIVE_CHECK_PHASES, type DatabaseCheckStatus } from '../types';

type Props = {
  open: boolean;
  status: DatabaseCheckStatus | null | undefined;
  cancelling: boolean;
  onCancel: () => void;
  onClose: () => void;
};

export function DatabaseCheckDialog({ open: isOpen, status: currentStatus, cancelling, onCancel, onClose }: Props) {
  const { t } = useTranslation('database');
  const running = currentStatus ? ACTIVE_CHECK_PHASES.includes(currentStatus.phase) : true;
  const percent =
    currentStatus && currentStatus.tableTotal > 0
      ? Math.round((currentStatus.tableIndex / currentStatus.tableTotal) * 100)
      : 0;

  return (
    <Dialog open={isOpen} onOpenChange={(next) => !next && !running && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('check.dialog.title')}</DialogTitle>
          <DialogDescription>{t('check.dialog.description')}</DialogDescription>
        </DialogHeader>

        <Stack gap={4}>
          <TypographyText size="sm" tone="muted">
            {currentStatus ? t(`check.phase.${currentStatus.phase}`) : t('check.phase.queued')}
          </TypographyText>

          <ProgressBar value={percent} />

          {currentStatus?.currentTable && (
            <TypographyText size="sm" className="truncate font-mono">
              {currentStatus.currentTable}
            </TypographyText>
          )}

          <Flex align="center" justify="end" gap={2}>
            {running ? (
              <Button variant="outline" disabled={cancelling} onClick={onCancel}>
                {t('check.dialog.cancel')}
              </Button>
            ) : (
              <Button onClick={onClose}>{t('check.dialog.close')}</Button>
            )}
          </Flex>
        </Stack>
      </DialogContent>
    </Dialog>
  );
}
