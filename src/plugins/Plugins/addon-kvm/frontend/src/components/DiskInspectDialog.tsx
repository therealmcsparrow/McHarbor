// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import { useInspectImage } from '../hooks/useResize';
import type { Disk, VM } from '../types';

type Operation = 'info' | 'check' | 'map' | 'measure';

const OPERATIONS: Operation[] = ['info', 'check', 'map', 'measure'];

/**
 * Read-only qemu-img diagnostics on one disk.
 *
 * Repair is offered only while the guest is stopped, and the agent refuses it
 * regardless: `qemu-img check -r` against a disk a live guest has open
 * corrupts it, and only the host can tell whether it is open.
 */
export function DiskInspectDialog({ vm, disk, onClose }: { vm: VM; disk: Disk; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const inspect = useInspectImage(vm.id);
  const [operation, setOperation] = useState<Operation>('info');

  const stopped = vm.state === 'stopped';

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('inspect.title', { slot: disk.slot })}</DialogTitle>
          <DialogDescription>{t('inspect.description')}</DialogDescription>
        </DialogHeader>

        <div className="flex items-end gap-2">
          <label className="flex-1 space-y-1">
            <span className="text-sm text-muted-foreground">{t('inspect.operation')}</span>
            <Select
              value={operation}
              ariaLabel={t('inspect.operation')}
              options={OPERATIONS.map((entry) => ({
                value: entry,
                label: t(`inspect.operations.${entry}`),
              }))}
              onChange={(value) => setOperation(value as Operation)}
            />
          </label>
          <Button
            permission={VM_PERM.MANAGE}
            disabled={inspect.isPending}
            onClick={() => inspect.mutate({ diskId: disk.id, operation })}
          >
            {t('inspect.run')}
          </Button>
        </div>

        {operation === 'check' && (
          <p className="text-sm text-muted-foreground">
            {stopped ? t('inspect.repairAvailable') : t('inspect.repairNeedsStop')}
          </p>
        )}

        {operation === 'check' && stopped && (
          <Button
            variant="outline"
            permission={VM_PERM.MANAGE}
            disabled={inspect.isPending}
            onClick={() =>
              inspect.mutate({
                diskId: disk.id,
                operation: 'check',
                repair: true,
              })
            }
          >
            {t('inspect.repair')}
          </Button>
        )}

        {inspect.data !== undefined && (
          <pre className="max-h-[320px] overflow-auto rounded-lg border border-border bg-muted p-3 font-mono text-xs text-foreground">
            {JSON.stringify(inspect.data, null, 2)}
          </pre>
        )}

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.close', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
