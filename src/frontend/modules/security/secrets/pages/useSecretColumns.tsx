// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { formatDate } from '@resources/utils/format';
import { SecretRowActions, type SecretRowHandlers } from '../components/SecretRowActions';
import { kindLabel } from '@resources/utils/secret-kind';
import { useSecretOptionalColumns } from './useSecretOptionalColumns';
import type { Secret } from '../types';

/**
 * The secrets table's columns, extracted so the page itself stays well under
 * 200 lines.
 *
 * There is no value column, and there is no way to add one: `Secret` has no
 * value field. Reading a value is the reveal dialog's job, and that costs an
 * elevation, a reason, and an audit entry.
 */
export function useSecretColumns({ onRotate, onReveal, onDelete, onReviewImpact, onEdit }: SecretRowHandlers) {
  const { t } = useTranslation('secrets');
  const { t: tc } = useTranslation('common');
  const optionalColumns = useSecretOptionalColumns();

  return useMemo<ColumnDef<Secret, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('columns.name'),
        cell: ({ row }) => (
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate font-mono font-medium">{row.original.name}</span>
            {row.original.system && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="secondary">{t('badges.system')}</Badge>
                </TooltipTrigger>
                <TooltipContent>{t('badges.systemHint', { module: row.original.originModule ?? '' })}</TooltipContent>
              </Tooltip>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'group',
        header: t('columns.group'),
        cell: ({ row }) =>
          row.original.group ? (
            <Badge variant="secondary" className="max-w-[12rem]" title={row.original.group}>
              <span className="truncate">{row.original.group}</span>
            </Badge>
          ) : (
            <span className="text-muted-foreground">{t('groups.ungrouped')}</span>
          ),
      },
      {
        accessorKey: 'kind',
        header: t('columns.kind'),
        cell: ({ row }) => kindLabel(t, row.original.kind),
      },
      {
        accessorKey: 'scope',
        header: t('columns.scope'),
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {t(`scope.${row.original.scope}`)}
            {row.original.stackName ? ` · ${row.original.stackName}` : ''}
          </span>
        ),
      },
      { accessorKey: 'version', header: t('columns.version') },
      {
        accessorKey: 'referenceCount',
        header: t('columns.references'),
        cell: ({ row }) =>
          row.original.referenceCount > 0 ? (
            <Badge variant="success">{row.original.referenceCount}</Badge>
          ) : (
            <Badge variant="warning">{t('badges.unused')}</Badge>
          ),
      },
      {
        accessorKey: 'staleCount',
        header: t('columns.stale'),
        cell: ({ row }) =>
          row.original.staleCount > 0 ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="warning">{row.original.staleCount}</Badge>
              </TooltipTrigger>
              <TooltipContent>
                {t('badges.staleHint', {
                  count: row.original.staleCount,
                  version: row.original.version,
                })}
              </TooltipContent>
            </Tooltip>
          ) : (
            <Badge variant="secondary">{t('badges.upToDate')}</Badge>
          ),
      },
      {
        accessorKey: 'lastUsedAt',
        header: t('columns.lastUsed'),
        cell: ({ row }) => (row.original.lastUsedAt ? formatDate(row.original.lastUsedAt) : tc('labels.never')),
      },
      {
        accessorKey: 'lastRevealedAt',
        header: t('columns.lastRevealed'),
        cell: ({ row }) => (row.original.lastRevealedAt ? formatDate(row.original.lastRevealedAt) : tc('labels.never')),
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('columns.actions')}</span>,
        meta: { label: t('columns.actions'), locked: true, noExport: true },
        cell: ({ row }) => (
          <SecretRowActions
            secret={row.original}
            onRotate={onRotate}
            onReveal={onReveal}
            onDelete={onDelete}
            onReviewImpact={onReviewImpact}
            onEdit={onEdit}
          />
        ),
      },
    ],
    [t, tc, optionalColumns, onReveal, onRotate, onDelete, onReviewImpact, onEdit],
  );
}
