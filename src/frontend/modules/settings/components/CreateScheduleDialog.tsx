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
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';

type CreateScheduleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateScheduleDialog({ open, onOpenChange }: CreateScheduleDialogProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const queryClient = useQueryClient();

  const createSchedule = useMutation({
    mutationFn: (data: { name: string; cron: string; action: string; target: string }) =>
      api.post('/schedules', data).then(assertSuccess),
    meta: { success: t('toast.scheduleCreated') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['schedules'] }),
  });

  const [schName, setSchName] = useState('');
  const [schCron, setSchCron] = useState('0 3 * * *');
  const [schAction, setSchAction] = useState('restart');
  const [schTarget, setSchTarget] = useState('');

  const handleCreate = () => {
    if (!schName.trim() || !schCron.trim() || !schTarget.trim()) return;
    createSchedule.mutate(
      {
        name: schName.trim(),
        cron: schCron.trim(),
        action: schAction,
        target: schTarget.trim(),
      },
      {
        onSuccess: () => {
          onOpenChange(false);
          setSchName('');
          setSchCron('0 3 * * *');
          setSchAction('restart');
          setSchTarget('');
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('schedules.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('schedules.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('schedules.nameLabel')}</Label>
            <Input
              type="text"
              value={schName}
              onChange={(e) => setSchName(e.target.value)}
              placeholder={t('schedules.namePlaceholder')}
            />
          </div>
          <ReadableScheduleField
            value={schCron}
            onChange={setSchCron}
            label={t('schedules.cronLabel')}
            hint={t('schedules.cronHint')}
          />
          <div>
            <Label className="mb-2">{t('schedules.actionLabel')}</Label>
            <Select
              value={schAction}
              onChange={setSchAction}
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
              value={schTarget}
              onChange={(e) => setSchTarget(e.target.value)}
              placeholder={t('schedules.targetPlaceholder')}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button
            onClick={handleCreate}
            disabled={
              createSchedule.isPending ||
              !schName.trim() ||
              !schCron.trim() ||
              !schTarget.trim()
            }
          >
            {createSchedule.isPending ? t('schedules.creating') : t('schedules.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
