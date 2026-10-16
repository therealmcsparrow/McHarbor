// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { cn } from '@resources/utils/cn';
import { ScheduleEditFields } from './ScheduleEditFields';
import { useScheduleEdit } from './useScheduleEdit';
import type { Schedule } from '../types';
import { useTypeName } from './catalog-labels';

// Editing a schedule that already exists.
//
// A dialog on the schedules page rather than a second pass through the
// studio: the studio's question is "which report", and a schedule that
// exists has answered it. What is left is everything about *how* it runs,
// which is what this asks.
//
// The report type is shown, not edited. It decides which sections, options
// and scopes are even legal, so changing it would silently invalidate three
// other fields — and a schedule that reports something else is a new
// schedule, not an edited one.

type Props = {
  schedule: Schedule | null;
  onClose: () => void;
};

export function ScheduleEditDialog({ schedule, onClose }: Props) {
  return (
    <Dialog
      open={schedule !== null}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) onClose();
      }}
    >
      {/* Keyed on the id so opening a different schedule rebuilds the form
          from that row, rather than carrying the previous one's edits. */}
      {schedule ? <EditForm key={schedule.id} schedule={schedule} onClose={onClose} /> : null}
    </Dialog>
  );
}

function EditForm({ schedule, onClose }: { schedule: Schedule; onClose: () => void }) {
  const { t } = useTranslation('reporting');
  const typeName = useTypeName();
  const state = useScheduleEdit(schedule, onClose);
  const { update, incomplete, submit } = state;

  return (
    <DialogContent className="max-w-2xl">
      <DialogHeader>
        <DialogTitle>{t('schedules.edit.title')}</DialogTitle>
        <DialogDescription>{typeName(schedule.report_type)}</DialogDescription>
      </DialogHeader>

      <DialogBody className="space-y-5">
        <ScheduleEditFields state={state} />
      </DialogBody>

      <DialogFooter className="items-center justify-between gap-3 sm:justify-between">
        <p className={cn('text-xs', update.isError || incomplete ? 'text-danger' : 'text-muted-foreground')}>
          {incomplete
            ? t('schedule.incomplete')
            : update.isError
              ? t('schedules.edit.error')
              : t('schedules.edit.hint')}
        </p>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onClose} disabled={update.isPending}>
            {t('action.cancel')}
          </Button>
          <Button permission="reporting.manage" onClick={submit} disabled={update.isPending || incomplete}>
            {update.isPending ? <Spinner className="size-4" /> : null}
            {t('action.save')}
          </Button>
        </div>
      </DialogFooter>
    </DialogContent>
  );
}
