// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { VM_PERM } from '../permissions';
import { useResizeDisk } from '../hooks/useVMHardware';
import { formatBytes } from '../format';
import type { Disk, VM } from '../types';

const GIB = 1024 ** 3;

export function DiskResizeDialog({ vm, disk, onClose }: { vm: VM; disk: Disk; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const resize = useResizeDisk();

  // The service refuses a size below the current one; the control mirrors that
  // so the floor is visible before a request is spent on it.
  const floorGb = Math.floor(disk.sizeBytes / GIB) + 1;
  const [sizeGb, setSizeGb] = useState(floorGb);

  const submit = () => {
    if (resize.isPending || sizeGb < floorGb) return;
    resize.mutate({ id: vm.id, diskId: disk.id, sizeGb }, { onSuccess: onClose });
  };

  return (
    <Dialog open onOpenChange={(isOpen) => !isOpen && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('hardware.resizeDisk.title')}</DialogTitle>
          <DialogDescription>
            {t('hardware.resizeDisk.current', { size: formatBytes(disk.sizeBytes) })}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2 py-2">
          <Label htmlFor="disk-resize-size">{t('hardware.resizeDisk.newSize')}</Label>
          <NumberInput id="disk-resize-size" min={floorGb} value={sizeGb} onChange={setSizeGb} />
          <p className="text-xs text-muted-foreground">{t('hardware.resizeDisk.growOnly')}</p>
          <p className="text-xs text-muted-foreground">{t('hardware.resizeDisk.afterNote')}</p>
          {resize.isError && (
            <p role="alert" className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {resize.error.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={resize.isPending || sizeGb < floorGb} onClick={submit}>
            {t('hardware.resizeDisk.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
