// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import type { Snapshot, VM } from '../types';

/**
 * Confirm a revert.
 *
 * Nothing is discarded any more — the branch the guest is leaving stays where
 * it is — but everything written since the snapshot goes, which is why the
 * guest's name is still typed out.
 */
export function RevertDialog({
  vm,
  snapshot,
  pending,
  restartAfter,
  onRestartAfterChange,
  onClose,
  onConfirm,
}: {
  vm: VM;
  snapshot: Snapshot;
  pending: boolean;
  restartAfter: boolean;
  onRestartAfterChange: (value: boolean) => void;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation('vm');
  const [typedName, setTypedName] = useState('');
  const running = vm.state === 'running';

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('snapshots.revert.title', { name: snapshot.name })}</DialogTitle>
          <DialogDescription>{t('snapshots.revert.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="rounded-md bg-muted p-3">
            <p className="text-sm text-muted-foreground">{t('snapshots.revert.branchKept')}</p>
          </div>

          {running && (
            <>
              <Alert variant="warning" description={t('snapshots.revert.willStop')} />
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="revert-restart">{t('snapshots.revert.restartAfter')}</Label>
                <Switch id="revert-restart" checked={restartAfter} onCheckedChange={onRestartAfterChange} />
              </div>
            </>
          )}

          <div>
            <Label htmlFor="revert-confirm">{t('snapshots.revert.confirmName', { name: vm.name })}</Label>
            <Input
              id="revert-confirm"
              value={typedName}
              onChange={(evt) => setTypedName(evt.target.value)}
              autoComplete="off"
              spellCheck={false}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button
            variant="destructive"
            permission={VM_PERM.MANAGE}
            disabled={typedName.trim() !== vm.name || pending}
            onClick={onConfirm}
          >
            {pending && <Spinner className="mr-2 h-4 w-4" />}
            {t('snapshots.revert.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
