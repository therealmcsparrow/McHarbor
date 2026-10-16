// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconClockPlus, IconPlayerPlay, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { describeCron } from '@resources/utils/schedule';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  useCreateSchedule,
  useDeleteSchedule,
  useRunSchedule,
  useSnapshotSchedules,
  useUpdateSchedule,
} from '../hooks/useSnapshotSchedules';
import { formatRelative } from '../format';
import { VM_PERM } from '../permissions';
import type { ScheduleInput, SnapshotSchedule, VM } from '../types';
import { ScheduleEditDialog } from './ScheduleEditDialog';
import { ScheduleRunHistory } from './ScheduleRunHistory';

const EMPTY: ScheduleInput = {
  name: '',
  cron: '0 2 * * *',
  enabled: true,
  keepCount: 7,
  withMemory: false,
  pauseWithoutAgent: true,
};

export function SnapshotSchedules({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const { t: common } = useTranslation('common');
  const { data, isLoading } = useSnapshotSchedules(vm.id);
  const create = useCreateSchedule();
  const update = useUpdateSchedule();
  const remove = useDeleteSchedule();
  const run = useRunSchedule();

  const [editing, setEditing] = useState<SnapshotSchedule | null>(null);
  const [draft, setDraft] = useState<ScheduleInput>(EMPTY);
  const [isOpen, setOpen] = useState(false);
  const [deleting, setDeleting] = useState<SnapshotSchedule | null>(null);

  if (isLoading) return <Spinner className="h-6 w-6" />;

  const schedules = data?.schedules ?? [];
  const runs = data?.runs ?? [];

  const openNew = () => {
    setEditing(null);
    setDraft({ ...EMPTY, name: t('schedules.defaultName') });
    setOpen(true);
  };

  const openEdit = (schedule: SnapshotSchedule) => {
    setEditing(schedule);
    setDraft({
      name: schedule.name,
      cron: schedule.cron,
      enabled: schedule.enabled,
      keepCount: schedule.keepCount,
      withMemory: schedule.withMemory,
      pauseWithoutAgent: schedule.pauseWithoutAgent,
    });
    setOpen(true);
  };

  const save = () => {
    if (editing) {
      update.mutate({ id: vm.id, scheduleId: editing.id, ...draft });
    } else {
      create.mutate({ id: vm.id, ...draft });
    }
    setOpen(false);
  };

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t('schedules.intro')}</p>
        <Button size="sm" permission={VM_PERM.MANAGE} disabled={!vm.hostAvailable} onClick={openNew}>
          <IconClockPlus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('schedules.add')}
        </Button>
      </header>

      {schedules.length === 0 ? (
        <p className="rounded-md border border-border p-4 text-sm text-muted-foreground">{t('schedules.empty')}</p>
      ) : (
        <ul className="space-y-2">
          {schedules.map((schedule) => (
            <li
              key={schedule.id}
              className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border p-3"
            >
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    variant="link"
                    size="lg"
                    className="h-auto truncate p-0 text-sm font-medium text-foreground underline-offset-2 hover:text-foreground"
                    onClick={() => openEdit(schedule)}
                  >
                    {schedule.name}
                  </Button>
                  <Badge variant={schedule.enabled ? 'default' : 'secondary'}>
                    {schedule.enabled ? t('schedules.enabled') : t('schedules.disabled')}
                  </Badge>
                  {schedule.withMemory ? <Badge variant="secondary">{t('schedules.withMemory')}</Badge> : null}
                </div>
                <p className="text-xs text-muted-foreground">
                  {describeCron(schedule.cron, common)}
                  <span className="ml-2 font-mono opacity-70">{schedule.cron}</span>
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('schedules.keeps', { count: schedule.keepCount })}
                  {schedule.nextRunAt ? ` · ${t('schedules.next', { when: formatRelative(schedule.nextRunAt) })}` : ''}
                </p>
                {schedule.lastError ? <p className="text-xs text-destructive">{schedule.lastError}</p> : null}
              </div>

              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  permission={VM_PERM.MANAGE}
                  aria-label={t('schedules.runNow')}
                  disabled={!vm.hostAvailable || run.isPending}
                  onClick={() => run.mutate({ id: vm.id, scheduleId: schedule.id })}
                >
                  <IconPlayerPlay className="h-4 w-4" aria-hidden="true" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  permission={VM_PERM.MANAGE}
                  aria-label={t('schedules.remove')}
                  onClick={() => setDeleting(schedule)}
                >
                  <IconTrash className="h-4 w-4" aria-hidden="true" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {runs.length > 0 ? <ScheduleRunHistory runs={runs} /> : null}

      <ScheduleEditDialog
        open={isOpen}
        onOpenChange={setOpen}
        editing={editing !== null}
        draft={draft}
        setDraft={setDraft}
        saving={create.isPending || update.isPending}
        onSave={save}
      />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(next) => {
          if (!next) setDeleting(null);
        }}
        title={t('schedules.confirmRemove.title')}
        description={t('schedules.confirmRemove.body')}
        confirmLabel={t('schedules.remove')}
        variant="destructive"
        onConfirm={() => {
          if (deleting) remove.mutate({ id: vm.id, scheduleId: deleting.id });
          setDeleting(null);
        }}
      />
    </div>
  );
}
