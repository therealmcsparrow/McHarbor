// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconClock } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { SearchMode } from '@resources/utils/search-filter';
import { useDeleteSchedule, useRunScheduleNow, useSchedules } from '../hooks/useReporting';
import { useScheduleColumns } from './ScheduleColumns';
import { ScheduleEditDialog } from './ScheduleEditDialog';
import { ScheduleRunDialog } from './ScheduleRunDialog';
import type { Schedule } from '../types';
import { useTypeName } from './catalog-labels';

// Report Schedules.
//
// Every saved schedule, when it next fires, where it delivers, and how its
// most recent run went. There is no history here on purpose: a schedule
// keeps exactly one run, so this page is a status board rather than a log.
//
// Schedules are created from the studio, where the report they run is
// chosen — this page manages the ones that exist. Sending a reader here to
// create one would mean asking every question the configure dialog already
// asks, in a second place, with a second chance to drift.

export function SchedulesPage() {
  const { t } = useTranslation('reporting');
  const typeName = useTypeName();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState<SearchMode>('contains');
  const [pendingDelete, setPendingDelete] = useState<Schedule | null>(null);
  const [openRun, setOpenRun] = useState<Schedule | null>(null);
  const [editing, setEditing] = useState<Schedule | null>(null);

  const schedules = useSchedules(page, pageSize);
  const remove = useDeleteSchedule();
  const runNow = useRunScheduleNow();

  const items = useMemo(() => schedules.data?.items ?? [], [schedules.data]);
  const total = schedules.data?.total ?? 0;
  const totalPages = Math.max(schedules.data?.total_pages ?? 1, 1);
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = items.length === 0 ? 0 : from + items.length - 1;

  const onRunNow = useCallback(
    (schedule: Schedule) => {
      runNow.mutate(schedule.id);
    },
    [runNow],
  );
  const onDelete = useCallback((schedule: Schedule) => setPendingDelete(schedule), []);
  const onEdit = useCallback((schedule: Schedule) => setEditing(schedule), []);

  const columns = useScheduleColumns({
    onRunNow,
    onEdit,
    onDelete,
    runningId: runNow.isPending ? (runNow.variables ?? null) : null,
  });

  return (
    <>
      <TablePage
        id="reporting-schedules"
        title={t('page.schedules.title')}
        description={t('page.schedules.subtitle')}
        search={{
          value: query,
          onChange: setQuery,
          mode,
          onModeChange: setMode,
          placeholder: t('schedules.searchPlaceholder'),
          fields: (schedule) => [
            schedule.name,
            schedule.environment,
            schedule.cron,
            typeName(schedule.report_type),
            ...schedule.targets.map((target) => target.name ?? ''),
          ],
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: {
            onRefresh: () => void schedules.refetch(),
            isFetching: schedules.isFetching,
          },
          export: { filename: 'mcharbor-report-schedules' },
        }}
        columns={columns}
        data={items}
        loading={schedules.isLoading}
        error={schedules.error}
        onRetry={() => void schedules.refetch()}
        empty={{
          icon: <IconClock className="size-8" />,
          title: query.trim() ? t('schedules.noMatches') : t('schedules.empty.title'),
          description: query.trim() ? undefined : t('schedules.empty.description'),
        }}
        getRowId={(schedule) => schedule.id}
        // Opening a row shows the last run; a schedule that has never run
        // opens its settings instead. Editing is also on the row's pencil,
        // so the click is a shortcut rather than the only way in — and a
        // row that did nothing at all would be the worse alternative.
        onRowClick={(schedule) => {
          if (schedule.last_run) setOpenRun(schedule);
          else setEditing(schedule);
        }}
        pagination={{
          mode: 'server',
          page,
          totalPages,
          pageSize,
          total,
          from,
          to,
          onPageChange: (next) => setPage(Math.min(Math.max(next, 1), totalPages)),
          onPageSizeChange: (next) => {
            setPageSize(next);
            setPage(1);
          },
        }}
      />

      <ScheduleRunDialog schedule={openRun} onClose={() => setOpenRun(null)} />

      <ScheduleEditDialog schedule={editing} onClose={() => setEditing(null)} />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) setPendingDelete(null);
        }}
        title={t('schedules.delete.title')}
        description={t('schedules.delete.description', { name: pendingDelete?.name ?? '' })}
        confirmLabel={t('action.delete')}
        variant="destructive"
        loading={remove.isPending}
        onConfirm={() => {
          if (pendingDelete) remove.mutate(pendingDelete.id);
          setPendingDelete(null);
        }}
      />
    </>
  );
}
