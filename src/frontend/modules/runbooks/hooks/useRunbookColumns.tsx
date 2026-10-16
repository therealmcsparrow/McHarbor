// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { describeCron } from '@resources/utils/schedule';
import { PERM } from '@core/auth/permissionKeys';
import type { Runbook } from '@resources/types/runbooks';
import { RunbookExportMenu } from '../components/RunbookExportMenu';
import { useAttachLabel } from './useAttachLabel';

type RunbookColumnActions = {
  onEdit: (id: string) => void;
  onRemove: (id: string) => void;
};

export function useRunbookColumns({ onEdit, onRemove }: RunbookColumnActions) {
  const { t } = useTranslation('runbooks');
  const { t: tCommon } = useTranslation('common');
  const attachLabel = useAttachLabel();

  return useMemo<ColumnDef<Runbook, unknown>[]>(
    () => [
      {
        id: 'name',
        header: t('list.columnName'),
        accessorFn: (row) => row.name,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium text-foreground">{row.original.name}</p>
            <p className="truncate text-xs text-muted-foreground">{row.original.description}</p>
          </div>
        ),
      },
      {
        id: 'attach',
        header: t('list.columnAttach'),
        accessorFn: (row) => row.attachKind,
        cell: ({ row }) => <Badge variant="secondary">{attachLabel(row.original.attachKind).label}</Badge>,
      },
      {
        id: 'steps',
        header: t('list.columnSteps'),
        accessorFn: (row) => row.steps.length,
        cell: ({ row }) => <Badge variant="outline">{t('list.stepCount', { count: row.original.steps.length })}</Badge>,
      },
      {
        id: 'automation',
        header: t('list.columnAutomation'),
        // Sorted and searched on the readable form, because "0 7 * * *" is not
        // what anyone types into a search box.
        accessorFn: (row) =>
          row.automation?.cron
            ? describeCron(row.automation.cron, tCommon)
            : row.automation?.onAlert
              ? t('list.automationOnAlert')
              : t('list.automationManual'),
        cell: ({ row }) => {
          const automation = row.original.automation;
          if (automation?.cron) {
            return (
              <div className="min-w-0">
                <p className="truncate text-sm">{describeCron(automation.cron, tCommon)}</p>
                <p className="truncate font-mono text-xs text-muted-foreground">{automation.cron}</p>
              </div>
            );
          }
          return (
            <span className="text-sm text-muted-foreground">
              {automation?.onAlert ? t('list.automationOnAlert') : t('list.automationManual')}
            </span>
          );
        },
      },
      {
        id: 'state',
        header: t('list.columnState'),
        accessorFn: (row) => (row.enabled ? 'enabled' : 'disabled'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Badge variant={row.original.enabled ? 'success' : 'secondary'}>
              {row.original.enabled ? t('list.enabled') : t('list.disabled')}
            </Badge>
            <StatusError error={row.original.automation?.lastError} />
          </div>
        ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex items-center justify-end gap-1.5">
            <Button
              variant="outline"
              size="sm"
              permission={PERM.RUNBOOKS_MANAGE}
              onClick={() => onEdit(row.original.id)}
            >
              {t('list.edit')}
            </Button>
            <RunbookExportMenu id={row.original.id} name={row.original.name} />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t('list.delete')}
              permission={PERM.RUNBOOKS_MANAGE}
              onClick={() => onRemove(row.original.id)}
            >
              <IconTrash className="size-4" aria-hidden="true" />
            </Button>
          </div>
        ),
      },
    ],
    // onEdit closes over setParams and onRemove over setPending, both stable for
    // the lifetime of the route, so the columns do not need to be rebuilt on
    // every search keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, tCommon, attachLabel],
  );
}
