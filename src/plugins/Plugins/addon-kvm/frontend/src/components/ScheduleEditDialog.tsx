// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

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
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { Switch } from '@resources/components/ui/Switch';
import { VM_PERM } from '../permissions';
import type { ScheduleInput } from '../types';

export function ScheduleEditDialog({
  open: isOpen,
  onOpenChange,
  editing,
  draft,
  setDraft,
  saving,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing: boolean;
  draft: ScheduleInput;
  setDraft: React.Dispatch<React.SetStateAction<ScheduleInput>>;
  saving: boolean;
  onSave: () => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{editing ? t('schedules.editTitle') : t('schedules.addTitle')}</DialogTitle>
          <DialogDescription>{t('schedules.dialogBody')}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="schedule-name">{t('schedules.field.name')}</Label>
            <Input
              id="schedule-name"
              value={draft.name}
              onChange={(evt) => setDraft({ ...draft, name: evt.target.value })}
            />
          </div>

          <ReadableScheduleField
            value={draft.cron}
            label={t('schedules.field.cron')}
            hint={t('schedules.field.cronHelp')}
            onChange={(cron) => setDraft((current) => ({ ...current, cron }))}
          />

          <div className="grid gap-1.5">
            <Label htmlFor="schedule-keep">{t('schedules.field.keep')}</Label>
            <NumberInput
              id="schedule-keep"
              min={1}
              max={100}
              value={draft.keepCount ?? 7}
              onChange={(keepCount) => setDraft({ ...draft, keepCount })}
            />
            <p className="text-xs text-muted-foreground">{t('schedules.field.keepHelp')}</p>
          </div>

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="schedule-memory">{t('schedules.field.memory')}</Label>
            <Switch
              id="schedule-memory"
              checked={draft.withMemory ?? false}
              onCheckedChange={(checked) => setDraft({ ...draft, withMemory: checked })}
            />
          </div>
          <p className="-mt-2 text-xs text-muted-foreground">{t('schedules.field.memoryHelp')}</p>

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="schedule-pause">{t('schedules.field.pause')}</Label>
            <Switch
              id="schedule-pause"
              checked={draft.pauseWithoutAgent ?? true}
              onCheckedChange={(checked) => setDraft({ ...draft, pauseWithoutAgent: checked })}
            />
          </div>
          <p className="-mt-2 text-xs text-muted-foreground">{t('schedules.field.pauseHelp')}</p>

          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="schedule-enabled">{t('schedules.field.enabled')}</Label>
            <Switch
              id="schedule-enabled"
              checked={draft.enabled ?? true}
              onCheckedChange={(checked) => setDraft({ ...draft, enabled: checked })}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={VM_PERM.MANAGE} disabled={!draft.cron.trim() || saving} onClick={onSave}>
            {t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
