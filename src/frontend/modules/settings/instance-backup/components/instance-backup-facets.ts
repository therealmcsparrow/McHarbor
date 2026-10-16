// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { ExportStatus, InstanceExport } from '../hooks/useInstanceBackups';
import type { ExportSchedule } from '../hooks/useExportSchedules';

export const FACET_ALL = 'all';

export type ExportTrigger = 'manual' | 'schedule';

export const EXPORT_TRIGGERS: readonly ExportTrigger[] = ['manual', 'schedule'];

const EXPORT_STATUSES: readonly ExportStatus[] = ['completed', 'running', 'failed'];

const STATUS_TONE: Record<ExportStatus, FilterFacet<string>['tone']> = {
  completed: 'success',
  running: 'info',
  failed: 'danger',
};

export function exportStatusFacets(rows: InstanceExport[], t: TFunction, tc: TFunction): FilterFacet<string>[] {
  return [
    { id: FACET_ALL, label: tc('labels.all'), count: rows.length },
    ...EXPORT_STATUSES.map((currentStatus) => ({
      id: currentStatus,
      label: t(`status.${currentStatus}`),
      count: rows.filter((row) => row.status === currentStatus).length,
      tone: STATUS_TONE[currentStatus],
    })),
  ];
}

export function filterExports(rows: InstanceExport[], currentStatus: string, trigger: string): InstanceExport[] {
  return rows.filter(
    (row) =>
      (currentStatus === FACET_ALL || row.status === currentStatus) &&
      (trigger === FACET_ALL || row.trigger === trigger),
  );
}

export type ScheduleFacet = typeof FACET_ALL | 'enabled' | 'disabled' | 'failing';

const SCHEDULE_MATCH: Record<Exclude<ScheduleFacet, typeof FACET_ALL>, (row: ExportSchedule) => boolean> = {
  enabled: (row) => row.enabled,
  disabled: (row) => !row.enabled,
  failing: (row) => row.lastStatus === 'failed',
};

export function scheduleFacets(rows: ExportSchedule[], t: TFunction, tc: TFunction): FilterFacet<string>[] {
  return [
    { id: FACET_ALL, label: tc('labels.all'), count: rows.length },
    {
      id: 'enabled',
      label: t('schedules.enabled'),
      count: rows.filter(SCHEDULE_MATCH.enabled).length,
      tone: 'success',
    },
    { id: 'disabled', label: t('schedules.disabled'), count: rows.filter(SCHEDULE_MATCH.disabled).length },
    {
      id: 'failing',
      label: t('schedules.facets.failing'),
      count: rows.filter(SCHEDULE_MATCH.failing).length,
      tone: 'danger',
    },
  ];
}

export function filterSchedules(rows: ExportSchedule[], facet: string): ExportSchedule[] {
  if (facet === FACET_ALL || !(facet in SCHEDULE_MATCH)) return rows;
  return rows.filter(SCHEDULE_MATCH[facet as keyof typeof SCHEDULE_MATCH]);
}
