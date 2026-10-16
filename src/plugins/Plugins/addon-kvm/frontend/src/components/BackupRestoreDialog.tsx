// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { Label } from '@resources/components/ui/Label';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import { useRestoreBackup, type BackupRun } from '../hooks/useVMBackups';
import { formatBytes, formatRelative } from '../format';
import type { VM } from '../types';

/**
 * Restore one backup into its machine's disks.
 *
 * Destructive and irreversible, so it is gated three ways: the machine has to
 * be stopped, the operator has to tick the consent box, and the server checks
 * the whole chain is present before a byte moves.
 */
export function BackupRestoreDialog({ vm, run, onClose }: { vm: VM; run: BackupRun; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const restore = useRestoreBackup(vm.id);
  const [confirmed, setConfirmed] = useState(false);

  const stopped = vm.state === 'stopped';
  // What the host has to find room for: the rebuilt disks plus the chain that
  // produces them, which is about twice the machine's disk size.
  const diskSize = formatBytes(
    (vm.disks ?? [])
      .filter((disk) => !disk.isCdrom && !disk.readonly)
      .reduce((total, disk) => total + (disk.sizeBytes ?? 0), 0) * 2,
  );
  const canRestore = stopped && confirmed && !restore.isPending;

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('backups.restoreDialog.title', {
              name: formatRelative(run.startedAt),
            })}
          </DialogTitle>
          <DialogDescription>{t('backups.restoreDialog.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {!stopped && (
            <p className="flex items-start gap-2 rounded-md bg-warning/10 p-3 text-sm text-foreground">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              {t('backups.restoreDialog.needsStop')}
            </p>
          )}

          {/*
            Named before they confirm, not after it fails. The assemble-beside
            strategy keeps the current disk intact until the very last step,
            and the price of that is room for a second copy on the host.
          */}
          <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
            {t('backups.restoreDialog.spaceNotice', { size: diskSize })}
          </p>

          {run.kind === 'incremental' && (
            <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
              {t('backups.restoreDialog.chainNote')}
            </p>
          )}

          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="restore-confirm">{t('backups.restoreDialog.confirm')}</Label>
            <Switch
              id="restore-confirm"
              checked={confirmed}
              disabled={!stopped}
              aria-label={t('backups.restoreDialog.confirm')}
              onCheckedChange={setConfirmed}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            variant="destructive"
            permission={VM_PERM.RESTORE_RUN}
            disabled={!canRestore}
            onClick={() => restore.mutate({ runId: run.id }, { onSuccess: onClose })}
          >
            {t('backups.restoreDialog.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
