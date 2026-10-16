// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Label } from '@resources/components/ui/Label';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { VM_PERM } from '../permissions';
import { useCreateBackupPlan, useUpdateBackupPlan, type BackupPlan } from '../hooks/useVMBackups';
import { useStorageLocations } from '../hooks/useStorageLocations';
import type { VM } from '../types';
import { BackupDestinationPicker } from './BackupDestinationPicker';

/**
 * Create or edit a backup schedule.
 *
 * The schedule is a ReadableScheduleField rather than a cron box: most people
 * setting a nightly backup do not know whether the day-of-week field is zero-
 * or one-based, and the ones who do still get it wrong first try.
 */
export function BackupPlanDialog({ vm, plan, onClose }: { vm: VM; plan?: BackupPlan; onClose: () => void }) {
  const { t } = useTranslation('vm');
  const locations = useStorageLocations();
  const create = useCreateBackupPlan(vm.id);
  const update = useUpdateBackupPlan(vm.id);

  const [nameValue, setName] = useState(plan?.name ?? '');
  const [cron, setCron] = useState(plan?.cron ?? '0 2 * * *');
  const [enabled, setEnabled] = useState(plan?.enabled ?? true);
  const [retentionCount, setRetentionCount] = useState(plan?.retentionCount ?? 7);
  const [retentionDays, setRetentionDays] = useState(plan?.retentionDays ?? 0);
  const [fullEvery, setFullEvery] = useState(plan?.fullEvery ?? 7);
  const [quiesce, setQuiesce] = useState(plan?.quiesce ?? true);
  const [destinationIds, setDestinationIds] = useState<string[]>(plan?.destinationIds ?? []);

  const available = locations.data ?? [];
  const pending = create.isPending || update.isPending;
  const canSave = nameValue.trim().length > 0 && destinationIds.length > 0 && !pending;

  function submit() {
    const input = {
      name: nameValue.trim(),
      cron,
      enabled,
      retentionCount,
      retentionDays,
      fullEvery,
      quiesce,
      destinationIds,
    };
    if (plan) {
      update.mutate({ planId: plan.id, input }, { onSuccess: onClose });
      return;
    }
    create.mutate(input, { onSuccess: onClose });
  }

  function toggleDestination(id: string) {
    setDestinationIds((current) => (current.includes(id) ? current.filter((entry) => entry !== id) : [...current, id]));
  }

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{plan ? t('backups.editPlan') : t('backups.addPlan')}</DialogTitle>
          <DialogDescription>{t('backups.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <div>
            <Label htmlFor="backup-name">{t('backups.form.name')}</Label>
            <Input
              id="backup-name"
              value={nameValue}
              placeholder={t('backups.form.namePlaceholder')}
              onChange={(evt) => setName(evt.target.value)}
            />
          </div>

          <ReadableScheduleField
            value={cron}
            label={t('backups.form.schedule')}
            hint={t('backups.form.scheduleHint')}
            onChange={setCron}
          />

          <div className="flex items-center justify-between">
            <Label>{t('backups.form.enabled')}</Label>
            <Switch checked={enabled} aria-label={t('backups.form.enabled')} onCheckedChange={setEnabled} />
          </div>

          <BackupDestinationPicker available={available} selectedIds={destinationIds} onToggle={toggleDestination} />

          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label>{t('backups.form.retentionCount')}</Label>
              <NumberInput
                value={retentionCount}
                min={0}
                max={1000}
                className="w-32"
                aria-label={t('backups.form.retentionCount')}
                onChange={setRetentionCount}
              />
            </div>
            <div>
              <Label>{t('backups.form.retentionDays')}</Label>
              <NumberInput
                value={retentionDays}
                min={0}
                max={3650}
                className="w-32"
                aria-label={t('backups.form.retentionDays')}
                onChange={setRetentionDays}
              />
            </div>
          </div>
          <p className="text-sm text-muted-foreground">{t('backups.form.retentionHint')}</p>

          <div>
            <Label>{t('backups.form.fullEvery')}</Label>
            <div className="flex items-center gap-2">
              <NumberInput
                value={fullEvery}
                min={0}
                max={365}
                className="w-32"
                aria-label={t('backups.form.fullEvery')}
                onChange={setFullEvery}
              />
              <span className="text-sm text-muted-foreground">{t('backups.form.fullEveryUnit')}</span>
            </div>
            <p className="mt-1 text-sm text-muted-foreground">{t('backups.form.fullEveryHint')}</p>
          </div>

          <div className="flex items-start justify-between gap-4">
            <div>
              <Label>{t('backups.form.quiesce')}</Label>
              <p className="text-sm text-muted-foreground">{t('backups.form.quiesceHint')}</p>
            </div>
            <Switch checked={quiesce} aria-label={t('backups.form.quiesce')} onCheckedChange={setQuiesce} />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={VM_PERM.BACKUP_MANAGE} disabled={!canSave} onClick={submit}>
            {t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
