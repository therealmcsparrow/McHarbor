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
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { VM_PERM } from '../permissions';
import { useDeleteVM, useVM } from '../hooks/useVMs';
import { formatBytes } from '../format';
import type { VM } from '../types';

/**
 * Destroy a guest.
 *
 * Two separate decisions again, and the second is the dangerous one. Removing
 * the machine leaves its disks in the pool, which is exactly what an operator
 * moving it to another host wants. Deleting the disks is permanent, so it takes
 * both a switch and the guest's name typed out.
 */
export function DeleteVMDialog({ vm, onClose, onDeleted }: { vm: VM; onClose: () => void; onDeleted: () => void }) {
  const { t } = useTranslation('vm');
  const remove = useDeleteVM();

  const [deleteVolumes, setDeleteVolumes] = useState(false);
  const [typedName, setTypedName] = useState('');
  // Latched once the removal is under way. The work continues on the host
  // whether or not this dialog is open, so from here the only honest control
  // is one that closes it.
  const [started, setStarted] = useState(false);
  // Polled only while the removal runs. The percentage arrives on the guest
  // row, so the dialog and the list are reading the same number rather than
  // each inventing one.
  const live = useVM(started ? vm.id : undefined);
  const progress = live.data?.deleteProgress;

  const totalDisk = (vm.disks ?? []).reduce((sum, disk) => sum + disk.sizeBytes, 0);
  // The name is required only for the destructive path. Demanding it for the
  // harmless one trains people to type it without reading.
  const confirmed = !deleteVolumes || typedName.trim() === vm.name;

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('delete.title', { name: vm.name })}</DialogTitle>
          <DialogDescription>{t('delete.body')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <Label htmlFor="vm-delete-volumes" className="text-sm font-normal">
                {t('delete.deleteVolumes')}
              </Label>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {deleteVolumes ? formatBytes(totalDisk) : t('delete.keepVolumesHint')}
              </p>
            </div>
            <Switch
              id="vm-delete-volumes"
              checked={deleteVolumes}
              onCheckedChange={(next) => {
                setDeleteVolumes(next);
                if (!next) setTypedName('');
              }}
            />
          </div>

          {deleteVolumes && (
            <div className="space-y-3 rounded-md border border-destructive/40 bg-destructive/5 p-3">
              <div className="flex items-start gap-2">
                <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" aria-hidden="true" />
                <p className="text-sm font-medium text-foreground">{t('delete.deleteVolumesWarning')}</p>
              </div>
              <div>
                <Label htmlFor="vm-confirm">{t('delete.confirmName', { name: vm.name })}</Label>
                <Input
                  id="vm-confirm"
                  value={typedName}
                  onChange={(evt) => setTypedName(evt.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
            </div>
          )}
        </div>

        {started && (
          <div className="space-y-2 rounded-md border border-border p-3">
            <div className="flex items-center gap-3">
              <Spinner className="h-4 w-4" />
              <div className="min-w-0 flex-1">
                <p className="text-sm text-foreground">
                  {progress ? t('delete.runningPercent', { percent: progress.percent }) : t('delete.running')}
                </p>
                <p className="text-xs text-muted-foreground">{t('delete.runningHint')}</p>
              </div>
            </div>
            {progress && <ProgressBar value={progress.percent} />}
          </div>
        )}

        <DialogFooter>
          {started ? (
            <Button
              variant="outline"
              onClick={() => {
                onClose();
                onDeleted();
              }}
            >
              {t('actions.close', { ns: 'common' })}
            </Button>
          ) : (
            <>
              <Button variant="outline" onClick={onClose}>
                {t('actions.cancel')}
              </Button>
              <Button
                variant="destructive"
                permission={VM_PERM.DELETE}
                disabled={!confirmed || remove.isPending}
                onClick={() => {
                  setStarted(true);
                  remove.mutate({ id: vm.id, deleteVolumes });
                }}
              >
                {remove.isPending && <Spinner className="mr-2 h-4 w-4" />}
                {t('delete.submit')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
