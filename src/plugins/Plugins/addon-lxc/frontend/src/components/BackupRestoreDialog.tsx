// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { CT_PERM } from '../permissions';
import { useRestoreBackup, type BackupRun } from '../hooks/useCTBackups';
import { formatAbsolute } from '../format';
import type { Container } from '../types';

export function BackupRestoreDialog({
  container,
  run,
  onOpenChange,
  onSuccess,
}: {
  container: Container;
  run: BackupRun;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
}) {
  const { t } = useTranslation('ct');
  const restore = useRestoreBackup(container.id);
  const [typedName, setTypedName] = useState('');

  const stopped = container.state !== 'running' && container.state !== 'frozen';
  const confirmed = typedName.trim() === container.name;
  const canRestore = stopped && confirmed && !restore.isPending;

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('backups.restoreDialog.title', { name: container.name })}</DialogTitle>
          <DialogDescription>
            {t('backups.restoreDialog.description', { taken: formatAbsolute(run.startedAt) })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {!stopped && (
            <p className="flex items-start gap-2 rounded-md bg-warning/10 p-3 text-sm text-foreground">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
              {t('backups.restoreDialog.needsStop')}
            </p>
          )}

          <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
            {t('backups.restoreDialog.spaceNotice')}
          </p>

          {run.kind === 'incremental' && (
            <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">
              {t('backups.restoreDialog.chainNote')}
            </p>
          )}

          <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 p-3">
            <div className="flex items-start gap-2">
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
              <p className="text-sm font-medium text-foreground">{t('backups.restoreDialog.irreversible')}</p>
            </div>
            <div>
              <Label htmlFor="ct-restore-confirm">{t('backups.restoreDialog.confirm', { name: container.name })}</Label>
              <Input
                id="ct-restore-confirm"
                value={typedName}
                disabled={!stopped}
                autoComplete="off"
                spellCheck={false}
                onChange={(ev) => setTypedName(ev.target.value)}
              />
            </div>
          </div>

          {restore.isError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {restore.error.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            variant="destructive"
            permission={CT_PERM.RESTORE}
            disabled={!canRestore}
            onClick={() =>
              restore.mutate(
                { runId: run.id, confirmName: typedName.trim() },
                {
                  onSuccess: () => {
                    onOpenChange(false);
                    onSuccess?.();
                  },
                },
              )
            }
          >
            {restore.isPending && <Spinner className="mr-2 h-4 w-4" />}
            {t('backups.restoreDialog.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
