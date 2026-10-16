// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useKindLabel } from '../hooks/usePluginKinds';
import type { UseMutationResult } from '@tanstack/react-query';
import type { ColumnDef } from '@resources/components/table-features';
import { StatusBadge, RECONCILER_STATUS } from '@resources/components/ui/StatusBadge';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { DesiredStateActions } from './DesiredStateActions';
import { timeAgo } from '@resources/utils/format';
import { type DesiredState, type DriftClass, deriveStatus } from '../hooks/useReconciler';
import { useReconcilerOptionalColumns } from './useReconcilerOptionalColumns';

type ColumnActions = {
  reconcile: UseMutationResult<unknown, Error, string>;
  checkDrift: UseMutationResult<unknown, Error, string>;
  onDelete: (id: string) => void;
  onShowDrift: (state: DesiredState) => void;
  onMode: (state: DesiredState) => void;
  onPropose: (state: DesiredState) => void;
  onPullRequest: (state: DesiredState) => void;
};

export function driftClasses(state: DesiredState): DriftClass[] {
  const seen = new Set<DriftClass>();
  for (const diff of state.lastReport?.diffs ?? []) seen.add(diff.class);
  return [...seen];
}

const MODE_VARIANT = { observe: 'secondary', propose: 'default', enforce: 'warning' } as const;

export function useReconcilerColumns({
  reconcile,
  checkDrift,
  onDelete,
  onShowDrift,
  onMode,
  onPropose,
  onPullRequest,
}: ColumnActions) {
  const { t } = useTranslation('common');
  const kindLabel = useKindLabel();
  const optionalColumns = useReconcilerOptionalColumns();

  return useMemo<ColumnDef<DesiredState, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('reconciler.columnName'),
        cell: ({ row }) => (
          <span className="flex items-center gap-2">
            <span className="font-medium">{row.original.name}</span>
            {row.original.sourceId && (
              <Badge variant="secondary" title={row.original.sourcePath}>
                {t('git', { ns: 'reconciler' })}
              </Badge>
            )}
          </span>
        ),
      },
      {
        accessorKey: 'mode',
        header: t('mode.column', { ns: 'reconciler' }),
        cell: ({ row }) => (
          <Badge variant={MODE_VARIANT[row.original.mode]}>
            {t(`mode.${row.original.mode}`, { ns: 'reconciler' })}
          </Badge>
        ),
      },
      {
        accessorKey: 'containerName',
        header: t('reconciler.columnContainer'),
        cell: ({ row }) => {
          const s = row.original;
          const label = s.source
            ? (s.subjectId ?? '')
            : s.kind === 'stack'
              ? s.stackName
              : s.kind === 'environment'
                ? t('target.environment', { ns: 'reconciler' })
                : s.containerName;
          return (
            <span className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
              <Badge variant="secondary">{kindLabel(s.kind)}</Badge>
              {label}
            </span>
          );
        },
      },
      {
        accessorKey: 'imageRef',
        header: t('reconciler.columnImage'),
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.imageRef}</span>,
      },
      {
        id: 'status',
        header: t('reconciler.columnStatus'),
        meta: { sortValue: deriveStatus },
        cell: ({ row }) => <StatusBadge status={deriveStatus(row.original)} map={RECONCILER_STATUS} />,
      },
      {
        id: 'lastReconciled',
        header: t('reconciler.columnLastReconciled'),
        meta: { sortValue: (s: DesiredState) => (s.lastReconcile ? Date.parse(s.lastReconcile) : null) },
        cell: ({ row }) => (row.original.lastReconcile ? timeAgo(row.original.lastReconcile) : t('reconciler.never')),
      },
      {
        id: 'driftDetected',
        header: t('reconciler.columnDrift'),
        meta: { sortValue: (s: DesiredState) => (s.driftDetected ? driftClasses(s).length : 0) },
        cell: ({ row }) => {
          const classes = driftClasses(row.original);
          if (!row.original.driftDetected || classes.length === 0) {
            return <span className="text-muted-foreground">{t('reconciler.driftNo')}</span>;
          }
          return (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto gap-1 px-1 py-0.5"
              aria-label={t('details.open', { ns: 'reconciler' })}
              onClick={() => onShowDrift(row.original)}
            >
              {classes.map((c) => (
                <Badge key={c} variant="warning">
                  {t(`class.${c}`, { ns: 'reconciler' })}
                </Badge>
              ))}
            </Button>
          );
        },
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('reconciler.columnActions')}</span>,
        meta: { locked: true, noExport: true, label: t('reconciler.columnActions') },
        cell: ({ row }) => (
          <DesiredStateActions
            state={row.original}
            reconcile={reconcile}
            checkDrift={checkDrift}
            onDelete={onDelete}
            onMode={onMode}
            onPropose={onPropose}
            onPullRequest={onPullRequest}
          />
        ),
      },
    ],
    [reconcile, checkDrift, onDelete, onShowDrift, onMode, onPropose, onPullRequest, t, optionalColumns, kindLabel],
  );
}
