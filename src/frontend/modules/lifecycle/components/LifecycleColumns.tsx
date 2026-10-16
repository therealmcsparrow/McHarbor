// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The lifecycle log's five columns.
//
// This surface used to be a `<ul>` of accordion rows: a chevron per row that
// expanded an inline detail panel. It is a real table now, and the detail panel
// became a modal — one row height, one scroll rhythm, and the same eye-icon
// affordance the audit and activity logs already use.

import { useMemo } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconEye, IconHistory } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { formatDate, type LocaleFormat } from '@resources/utils/format';
import { severityStyle } from '@resources/utils/severity';
import { cn } from '@resources/utils/cn';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { textColumn } from '@resources/layout/table-page/optional-columns';
import type { LifecycleEvent } from '../hooks/useLifecycle';
import { useLifecycleSubjectPages } from '../hooks/useLifecycleSubjectPages';
import {
  SEVERITY_LABEL,
  SUBJECT_ICON,
  buildSubjectPath,
  humanizeAction,
  type EnvironmentLite,
} from './lifecycle-format';

type UseLifecycleColumnsArgs = {
  formatPrefs: LocaleFormat;
  environments: EnvironmentLite[];
  onSelect: (event: LifecycleEvent) => void;
};

export function useLifecycleColumns({
  formatPrefs,
  environments,
  onSelect,
}: UseLifecycleColumnsArgs): ColumnDef<LifecycleEvent, unknown>[] {
  const { t } = useTranslation('settings');
  const subjectPages = useLifecycleSubjectPages();
  const group = t('lifecycle.columnsGroup');
  const environmentGroup = t('lifecycle.columnsGroupEnvironment');
  const envNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const env of environments) map.set(env.id, env.name);
    return map;
  }, [environments]);

  return useMemo(
    () => [
      {
        id: 'timestamp',
        accessorFn: (evt2) => formatDate(evt2.timestamp, formatPrefs),
        header: t('lifecycle.headerTime'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-help font-mono text-xs whitespace-nowrap text-muted-foreground">
                {formatDate(row.original.timestamp, formatPrefs)}
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('lifecycle.source', { source: row.original.source })}</TooltipContent>
          </Tooltip>
        ),
      },
      {
        id: 'severity',
        accessorFn: (evt2) => evt2.severity,
        header: t('lifecycle.headerSeverity'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const style = severityStyle(row.original.severity);
          const label = t(SEVERITY_LABEL[row.original.severity] ?? 'lifecycle.severity.info');
          return (
            <span
              className={cn(
                'inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-medium capitalize',
                style.pill,
                style.text,
              )}
              aria-label={label}
            >
              <span className={cn('size-1.5 rounded-full', style.dot)} aria-hidden="true" />
              {label}
            </span>
          );
        },
      },
      {
        id: 'subject',
        accessorFn: (entry) => entry.subjectName ?? entry.subjectId,
        header: t('lifecycle.headerSubject'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const eventItem = row.original;
          const SubjectIcon = SUBJECT_ICON[eventItem.subjectType] ?? IconHistory;
          const envName =
            environments.find((e) => e.id === eventItem.environmentId)?.name ??
            eventItem.environmentId ??
            t('lifecycle.localEnvironment');
          return (
            <div className="flex items-center gap-2">
              <SubjectIcon className="size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <Link
                  to={buildSubjectPath(eventItem, subjectPages)}
                  className="block truncate text-foreground hover:text-primary"
                  title={eventItem.subjectName ?? eventItem.subjectId}
                  onClick={(e) => e.stopPropagation()}
                >
                  {eventItem.subjectName ?? eventItem.subjectId.slice(0, 12)}
                </Link>
                <div className="truncate text-xs text-muted-foreground">
                  {eventItem.subjectType} · {envName}
                </div>
              </div>
            </div>
          );
        },
      },
      {
        id: 'action',
        accessorFn: (evt) => evt.action,
        header: t('lifecycle.headerDetail'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="truncate font-medium">{humanizeAction(row.original.action, t)}</div>
            <div className="truncate text-xs text-muted-foreground">
              {row.original.state
                ? t(`lifecycle.state.${row.original.state}`, { ns: 'common' })
                : row.original.eventType}
            </div>
          </div>
        ),
      },
      textColumn<LifecycleEvent>('subjectType', t('lifecycle.columnSubjectType'), (r) => r.subjectType, { group }),
      textColumn<LifecycleEvent>('subjectId', t('lifecycle.columnSubjectId'), (r) => r.subjectId, { group }),
      textColumn<LifecycleEvent>('eventType', t('lifecycle.columnEventType'), (r) => r.eventType, { group }),
      textColumn<LifecycleEvent>('state', t('lifecycle.columnState'), (r) => r.state, { group, width: 'sm' }),
      textColumn<LifecycleEvent>('source', t('lifecycle.columnSource'), (r) => r.source, { group, width: 'sm' }),
      textColumn<LifecycleEvent>(
        'environment',
        t('lifecycle.columnEnvironment'),
        (r) => r.environmentName ?? envNameById.get(r.environmentId ?? ''),
        { group: environmentGroup },
      ),
      {
        id: 'actions',
        header: t('lifecycle.detailsTitle'),
        enableSorting: false,
        meta: {
          width: 'xs',
          locked: true,
          noExport: true,
          label: t('lifecycle.detailsTitle'),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon-sm"
                  onClick={() => onSelect(row.original)}
                  aria-label={t('lifecycle.viewDetails')}
                >
                  <IconEye className="size-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('lifecycle.viewDetails')}</TooltipContent>
            </Tooltip>
          </div>
        ),
      },
    ],
    [environments, formatPrefs, onSelect, t, group, environmentGroup, envNameById, subjectPages],
  );
}
