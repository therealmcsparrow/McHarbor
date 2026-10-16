// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The two info modals the Backups overview opens from the runs table: what a
// run contained (the info button beside the type badge) and why it failed (the
// info button beside a failed status badge).

import { useTranslation } from 'react-i18next';
import { IconCheck, IconMinus } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
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
import { backupItemLabel, runTypeKey } from '../pages/backups-page-helpers';
import { itemCovered } from './run-item-coverage';

// ItemRow is one line in the contents list: a tick for what the run holds, a
// dash for what it skipped.
function ItemRow({ label, included }: { label: string; included: boolean }) {
  const { t } = useTranslation('containers');
  return (
    <Flex align="center" gap={2} className="py-1">
      {included ? (
        <IconCheck className="size-4 shrink-0 text-success" aria-hidden="true" />
      ) : (
        <IconMinus className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      )}
      <BodyText size="sm" tone={included ? 'default' : 'muted'}>
        {label}
      </BodyText>
      {!included && (
        <BodyText size="xs" tone="muted" className="ml-auto">
          {t('backups.itemsDialog.skipped')}
        </BodyText>
      )}
    </Flex>
  );
}

// BackupRunItemsDialog lists what a run captured or restored. Items the run was
// offered but left out are still listed, greyed and marked skipped — "what is
// in this backup" is only answerable next to what could have been.
export function BackupRunItemsDialog({ run, onClose }: { run: ContainerBackupRun | null; onClose: () => void }) {
  const { t } = useTranslation('containers');

  const included = run?.includedItems ?? [];
  const available = run?.availableItems ?? [];
  // Show the offered set when the server recorded one, so skipped items stay
  // visible; otherwise the captured set is all there is to show.
  const rows = (available.length > 0 ? available : included).map((item) => ({
    item,
    // A restored folder or mount covers what is inside it, as the restore did.
    included: itemCovered(item, included),
  }));
  // An item captured but never advertised (an archive entry from an older
  // container shape) still belongs in the list.
  for (const item of included) {
    if (!rows.some((row) => row.item === item)) {
      rows.push({ item, included: true });
    }
  }

  const isRestore = run?.operation === 'restore';

  return (
    <Dialog
      open={!!run}
      onOpenChange={(isOpen) => {
        if (!isOpen) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isRestore ? t('backups.itemsDialog.restoreTitle') : t('backups.itemsDialog.backupTitle')}
          </DialogTitle>
          <DialogDescription>
            {isRestore ? t('backups.itemsDialog.restoreDescription') : t('backups.itemsDialog.backupDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          <Flex align="center" gap={2}>
            <Badge variant="secondary">{run ? t(runTypeKey(run)) : null}</Badge>
            <BodyText size="sm" tone="muted">
              {run?.containerName || run?.containerId.slice(0, 12)}
            </BodyText>
          </Flex>
          {rows.length === 0 ? (
            <BodyText size="sm" tone="muted">
              {t('backups.itemsDialog.empty')}
            </BodyText>
          ) : (
            <Box className="divide-y divide-hairline rounded-md border border-border px-3 py-1">
              {rows.map((row) => (
                <ItemRow key={row.item} label={backupItemLabel(row.item, t)} included={row.included} />
              ))}
            </Box>
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

// The failure modal lives in its own file; re-exported so callers keep one
// import for both dialogs.
export { BackupRunErrorDialog } from './BackupRunErrorDialog';
