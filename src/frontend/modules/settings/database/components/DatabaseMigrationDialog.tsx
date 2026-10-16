// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowRight, IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { ACTIVE_MIGRATION_PHASES, type MigrationPhase, type MigrationStatus } from '../types';
import { DatabaseMigrationOutcome } from './DatabaseMigrationOutcome';
import { DatabaseMigrationTables } from './DatabaseMigrationTables';

/** The phases shown as chips across the top of the modal. */
const PHASE_CHIPS: MigrationPhase[] = [
  'prepare',
  'schema',
  'inventory',
  'copy',
  'verify',
  'catchup',
  'persist',
  'restart',
];

type DatabaseMigrationDialogProps = {
  open: boolean;
  status: MigrationStatus | null | undefined;
  onClose: () => void;
  onCancel: () => void;
  cancelling?: boolean;
};

/**
 * The progress modal.
 *
 * It is deliberately not dismissible while the job is live. Closing it would
 * not stop the copy — the job runs on the server — but it would leave the
 * operator with no way back to a screen that is, for the next minute or two,
 * the only place the application's state is being explained.
 */
export function DatabaseMigrationDialog({
  open: isOpen,
  status: currentStatus,
  onClose,
  onCancel,
  cancelling,
}: DatabaseMigrationDialogProps) {
  const { t } = useTranslation('database');

  const phase = currentStatus?.phase ?? 'queued';
  const active = ACTIVE_MIGRATION_PHASES.includes(phase);
  const percent =
    currentStatus && currentStatus.rowsTotal > 0
      ? Math.min(100, Math.round((currentStatus.rowsCopied / currentStatus.rowsTotal) * 100))
      : 0;

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => {
        // Only a finished job may be dismissed by clicking away.
        if (!next && !active) onClose();
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('migration.title')}</DialogTitle>
          <DialogDescription>
            {currentStatus ? (
              <Flex align="center" gap={2} wrap>
                <Badge variant="secondary">{t(`driver.${currentStatus.fromDriver}.name`)}</Badge>
                <IconArrowRight className="size-4" aria-hidden />
                <Badge variant="default">{t(`driver.${currentStatus.toDriver}.name`)}</Badge>
                <TypographyText size="xs" tone="muted" className="font-mono break-all">
                  {currentStatus.toTarget}
                </TypographyText>
              </Flex>
            ) : (
              t('migration.starting')
            )}
          </DialogDescription>
        </DialogHeader>

        <Stack gap={4} className="max-h-[60vh] overflow-y-auto">
          <Flex gap={2} wrap>
            {PHASE_CHIPS.map((chip) => {
              const index = PHASE_CHIPS.indexOf(chip);
              const currentIndex = PHASE_CHIPS.indexOf(phase);
              const reached = phase === 'done' || (currentIndex >= 0 && index <= currentIndex);
              return (
                <Badge key={chip} variant={chip === phase ? 'default' : reached ? 'success' : 'outline'}>
                  {t(`migration.phase.${chip}`)}
                </Badge>
              );
            })}
          </Flex>

          <DatabaseMigrationOutcome phase={phase} status={currentStatus} percent={percent} />

          <DatabaseMigrationTables status={currentStatus} />
        </Stack>

        <DialogFooter>
          {active ? (
            <Button variant="outline" permission={PERM.DATABASE_MANAGE} disabled={cancelling} onClick={onCancel}>
              {cancelling ? t('migration.cancelling') : t('migration.cancel')}
            </Button>
          ) : (
            <Button onClick={onClose}>
              {phase === 'done' && currentStatus?.restartScheduled ? (
                <>
                  <IconRefresh className="size-4" aria-hidden />
                  {t('migration.closeRestarting')}
                </>
              ) : (
                t('migration.close')
              )}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
