// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { PERM } from '@core/auth/permissionKeys';
import type { ScheduleItem } from '../types';

const DEFAULT_CRON = '0 3 * * *';

type ScheduleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The schedule being edited, or undefined to create a new one. */
  schedule?: ScheduleItem;
};

/**
 * ScheduleDialog creates a schedule and edits an existing one.
 *
 * One dialog rather than two, because the fields, their validation and the cron
 * builder are identical — a second near-identical dialog is what the duplication
 * check reports, and it is how the two drift until only one of them gets a new
 * field.
 */
export function ScheduleDialog({ open: isOpen, onOpenChange, schedule }: ScheduleDialogProps) {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();
  const editing = schedule !== undefined;

  const [scheduleName, setName] = useState('');
  const [description, setDescription] = useState('');
  const [cron, setCron] = useState(DEFAULT_CRON);
  const [action, setAction] = useState('restart');
  const [target, setTarget] = useState('');
  const [enabled, setEnabled] = useState(true);

  // Load the schedule's values when the dialog opens, and clear them when it
  // opens for a new one. Keyed on open as well as the id so reopening on the
  // same row discards an abandoned edit rather than resuming it.
  const [seeded, setSeeded] = useState<{ isOpen: boolean; schedule: typeof schedule | null }>({
    isOpen: false,
    schedule: null,
  });
  if (seeded.isOpen !== isOpen || seeded.schedule !== schedule) {
    setSeeded({ isOpen, schedule });
    if (isOpen) {
      setName(schedule?.name ?? '');
      setDescription(schedule?.description ?? '');
      setCron(schedule?.cron ?? DEFAULT_CRON);
      setAction(schedule?.action ?? 'restart');
      setTarget(schedule?.target ?? '');
      setEnabled(schedule?.enabled ?? true);
    }
  }

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      (editing ? api.put(`/schedules/${schedule.id}`, body) : api.post('/schedules', body)).then(assertSuccess),
    meta: {
      success: editing ? t('toast.scheduleUpdated') : t('toast.scheduleCreated'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['schedules'] });
      onOpenChange(false);
    },
  });

  const incomplete = !scheduleName.trim() || !cron.trim() || !target.trim();

  const submit = () => {
    if (incomplete) return;
    save.mutate({
      name: scheduleName.trim(),
      description: description.trim(),
      cron: cron.trim(),
      action,
      target: target.trim(),
      // Only the update route accepts it; a create starts active.
      ...(editing ? { enabled } : {}),
    });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{editing ? t('schedules.editDialogTitle') : t('schedules.dialogTitle')}</DialogTitle>
          <DialogDescription>
            {editing ? t('schedules.editDialogDescription') : t('schedules.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('schedules.nameLabel')}</Label>
            <Input
              type="text"
              value={scheduleName}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('schedules.namePlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('schedules.descriptionLabel')}</Label>
            <Input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('schedules.descriptionPlaceholder')}
            />
          </div>
          <ReadableScheduleField
            value={cron}
            onChange={setCron}
            label={t('schedules.cronLabel')}
            hint={t('schedules.cronHint')}
          />
          <div>
            <Label className="mb-2">{t('schedules.actionLabel')}</Label>
            <Select
              value={action}
              onChange={setAction}
              options={[
                { value: 'start', label: t('schedules.actionStart') },
                { value: 'stop', label: t('schedules.actionStop') },
                { value: 'restart', label: t('schedules.actionRestart') },
                { value: 'exec', label: t('schedules.actionExec') },
              ]}
            />
          </div>
          <div>
            <Label className="mb-2">{t('schedules.targetLabel')}</Label>
            <Input
              type="text"
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder={t('schedules.targetPlaceholder')}
            />
          </div>
          {editing && (
            <div className="flex items-start justify-between gap-4 rounded-lg border border-border p-3">
              <div className="min-w-0">
                <Label className="mb-1">{t('schedules.enabledLabel')}</Label>
                <p className="text-xs text-muted-foreground">{t('schedules.enabledHint')}</p>
              </div>
              <Switch checked={enabled} onCheckedChange={setEnabled} aria-label={t('schedules.enabledLabel')} />
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.SCHEDULES_MANAGE} onClick={submit} disabled={save.isPending || incomplete}>
            {save.isPending
              ? editing
                ? t('schedules.saving')
                : t('schedules.creating')
              : editing
                ? t('schedules.save')
                : t('schedules.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
