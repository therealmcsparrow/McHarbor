// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCalendarEvent, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Alert } from '@resources/components/ui/Alert';
import { TablePage } from '@resources/layout/TablePage';
import { describeCron } from '@resources/utils/schedule';
import type { SearchMode } from '@resources/utils/search-filter';
import { PERM } from '@core/auth/permissionKeys';
import {
  useDeleteSchedule,
  useExportSchedules,
  useRunSchedule,
  type ExportSchedule,
} from '../hooks/useExportSchedules';
import { ScheduleDialog } from './ScheduleDialog';
import { useScheduleColumns } from './ScheduleColumns';
import { FACET_ALL, filterSchedules, scheduleFacets } from './instance-backup-facets';

export function ScheduleList() {
  const { t } = useTranslation('instancebackup');
  const { t: tc } = useTranslation('common');
  const schedules = useExportSchedules();
  const run = useRunSchedule();
  const remove = useDeleteSchedule();
  const [editing, setEditing] = useState<ExportSchedule | null | undefined>(undefined);
  const [deleting, setDeleting] = useState<ExportSchedule | null>(null);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');
  const [facet, setFacet] = useState<string>(FACET_ALL);

  const rows = useMemo(() => schedules.data ?? [], [schedules.data]);
  const facets = useMemo(() => scheduleFacets(rows, t, tc), [rows, t, tc]);
  const visible = useMemo(() => filterSchedules(rows, facet), [rows, facet]);
  const runMutate = run.mutate;
  const runNow = useCallback((s: ExportSchedule) => runMutate(s.id), [runMutate]);
  const columns = useScheduleColumns({
    running: run.isPending,
    onRun: runNow,
    onEdit: setEditing,
    onDelete: setDeleting,
  });

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-sm font-semibold text-foreground">{t('schedules.title')}</h3>
      <TablePage
        id="instance-backup-schedules"
        header={false}
        title={t('schedules.title')}
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('schedules.search'),
          fields: (s) => [s.name, s.cron, describeCron(s.cron, tc), s.createdBy, s.lastError],
        }}
        facets={{ items: facets, value: facet, onChange: setFacet }}
        savedFilters={{
          state: { query, mode: searchMode, facet },
          onRestore: (s) => {
            setQuery(typeof s.query === 'string' ? s.query : '');
            if (s.mode === 'contains' || s.mode === 'exact' || s.mode === 'regex') setSearchMode(s.mode);
            setFacet(typeof s.facet === 'string' ? s.facet : FACET_ALL);
          },
          pristine: query === '' && facet === FACET_ALL,
          defaults: { query: '', mode: 'contains', facet: FACET_ALL },
        }}
        toolbar={{
          columns: true,
          density: true,
          refresh: { onRefresh: () => void schedules.refetch(), isFetching: schedules.isFetching },
          export: { filename: 'mcharbor-instance-export-schedules' },
          actions: (
            <Button
              variant="outline"
              size="sm"
              permission={PERM.INSTANCE_BACKUP_MANAGE}
              onClick={() => setEditing(null)}
            >
              <IconPlus className="size-4" />
              {t('schedules.add')}
            </Button>
          ),
        }}
        columns={columns}
        data={visible}
        loading={schedules.isLoading}
        empty={{ icon: <IconCalendarEvent className="size-8" />, title: t('schedules.empty') }}
        getRowId={(s) => s.id}
        getRowTestId={(s) => `schedule-row-${s.id}`}
      />
      {editing !== undefined && (
        <ScheduleDialog open schedule={editing} onOpenChange={(isOpen) => !isOpen && setEditing(undefined)} />
      )}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(isOpen) => {
          if (isOpen) return;
          setDeleting(null);
          remove.reset();
        }}
        title={t('schedules.deleteTitle')}
        description={t('schedules.deleteDescription', { name: deleting?.name ?? '' })}
        confirmLabel={t('schedules.deleteConfirm')}
        variant="destructive"
        loading={remove.isPending}
        permission={PERM.INSTANCE_BACKUP_MANAGE}
        onConfirm={() => deleting && remove.mutate(deleting.id, { onSuccess: () => setDeleting(null) })}
      >
        {remove.error && <Alert variant="destructive">{remove.error.message}</Alert>}
      </ConfirmDialog>
    </div>
  );
}
