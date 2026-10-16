// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import { useSetThrottle } from '../hooks/useResize';
import type { Disk, VM } from '../types';

const MB = 1024 * 1024;

/**
 * A disk's I/O limits.
 *
 * Bandwidth is entered in MB/s rather than bytes: the stored unit is bytes per
 * second, and asking an operator to type 209715200 is how a limit ends up an
 * order of magnitude off. Zero everywhere means unlimited, which is the
 * default and what an emptied field means.
 */
export function DiskLimitsDialog({ vm, disk, onClose }: { vm: VM; disk: Disk; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const setThrottle = useSetThrottle(vm.id);

  const [readIops, setReadIops] = useState(String(disk.readIops ?? 0));
  const [writeIops, setWriteIops] = useState(String(disk.writeIops ?? 0));
  const [readMbps, setReadMbps] = useState(String(Math.round((disk.readBps ?? 0) / MB)));
  const [writeMbps, setWriteMbps] = useState(String(Math.round((disk.writeBps ?? 0) / MB)));

  function submit() {
    setThrottle.mutate(
      {
        diskId: disk.id,
        limits: {
          readIops: Number(readIops) || 0,
          writeIops: Number(writeIops) || 0,
          readBps: (Number(readMbps) || 0) * MB,
          writeBps: (Number(writeMbps) || 0) * MB,
        },
      },
      { onSuccess: onClose },
    );
  }

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('limits.title', { slot: disk.slot })}</DialogTitle>
          <DialogDescription>{t('limits.description')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-3 sm:grid-cols-2">
          <LimitField label={t('limits.readIops')} value={readIops} onChange={setReadIops} />
          <LimitField label={t('limits.writeIops')} value={writeIops} onChange={setWriteIops} />
          <LimitField label={t('limits.readMbps')} value={readMbps} onChange={setReadMbps} />
          <LimitField label={t('limits.writeMbps')} value={writeMbps} onChange={setWriteMbps} />
        </div>

        <p className="text-sm text-muted-foreground">{t('limits.zeroHint')}</p>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={setThrottle.isPending} onClick={submit}>
            {t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function LimitField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label className="space-y-1">
      <span className="text-sm text-muted-foreground">{label}</span>
      <NumberInput
        nullable
        min={0}
        value={value.trim() === '' ? null : Number(value)}
        onChange={(next) => onChange(next === null ? '' : String(next))}
      />
    </label>
  );
}
