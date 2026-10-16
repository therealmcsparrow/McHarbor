// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { CT_PERM } from '../permissions';
import { useCreateBackupPlan, useUpdateBackupPlan, type BackupPlan } from '../hooks/useCTBackups';
import { useStorageLocations } from '../hooks/useStorageLocations';
import type { Container } from '../types';

export function BackupPlanDialog({
  container,
  plan,
  onOpenChange,
}: {
  container: Container;
  plan?: BackupPlan;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation('ct');
  const locations = useStorageLocations();
  const create = useCreateBackupPlan(container.id);
  const update = useUpdateBackupPlan(container.id);

  const [planName, setPlanName] = useState(plan?.name ?? container.name);
  const [cron, setCron] = useState(plan?.cron ?? '0 2 * * *');
  const [enabled, setEnabled] = useState(plan?.enabled ?? true);
  const [retentionCount, setRetentionCount] = useState(plan?.retentionCount ?? 7);
  const [retentionDays, setRetentionDays] = useState(plan?.retentionDays ?? 0);
  const [fullEvery, setFullEvery] = useState(plan?.fullEvery ?? 7);
  const [destinationIds, setDestinationIds] = useState<string[]>(plan?.destinationIds ?? []);

  const available = locations.data ?? [];
  const pending = create.isPending || update.isPending;
  const canSave = planName.trim().length > 0 && destinationIds.length > 0 && !pending;
  const failure = create.error ?? update.error;

  function submit() {
    const input = {
      name: planName.trim(),
      cron,
      enabled,
      retentionCount,
      retentionDays,
      fullEvery,
      destinationIds,
    };
    if (plan) {
      update.mutate({ planId: plan.id, input }, { onSuccess: () => onOpenChange(false) });
      return;
    }
    create.mutate(input, { onSuccess: () => onOpenChange(false) });
  }

  function toggleDestination(id: string) {
    setDestinationIds((current) => (current.includes(id) ? current.filter((entry) => entry !== id) : [...current, id]));
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{plan ? t('backups.editPlan') : t('backups.addPlan')}</DialogTitle>
          <DialogDescription>{t('backups.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-5">
          <div>
            <Label htmlFor="ct-backup-name">{t('backups.form.name')}</Label>
            <Input
              id="ct-backup-name"
              value={planName}
              placeholder={t('backups.form.namePlaceholder')}
              onChange={(ev) => setPlanName(ev.target.value)}
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

          <div>
            <Label>{t('backups.form.destinations')}</Label>
            <p className="mb-2 text-sm text-muted-foreground">{t('backups.form.destinationsHint')}</p>
            {available.length === 0 ? (
              <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">{t('backups.noDestinations')}</p>
            ) : (
              <div className="space-y-2">
                {available.map((destination) => (
                  <label key={destination.id} className="flex items-center justify-between gap-2">
                    <span className="text-sm text-foreground">
                      {destination.name}
                      <span className="ml-2 text-xs text-muted-foreground">{destination.locationType}</span>
                    </span>
                    <Switch
                      checked={destinationIds.includes(destination.id)}
                      aria-label={destination.name}
                      onCheckedChange={() => toggleDestination(destination.id)}
                    />
                  </label>
                ))}
              </div>
            )}
          </div>

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

          <p className="rounded-md bg-muted p-3 text-sm text-muted-foreground">{t('backups.form.quiesceNotice')}</p>

          {failure && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {failure.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button permission={CT_PERM.BACKUP} disabled={!canSave} onClick={submit}>
            {pending && <Spinner className="mr-2 h-4 w-4" />}
            {t('backups.form.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
