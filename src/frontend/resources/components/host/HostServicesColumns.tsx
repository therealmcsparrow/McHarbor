// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconShieldLock } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { StatusError } from '@resources/components/ui/StatusError';
import type { HostService, HostServiceAction } from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { HostServiceActionsCell } from './HostServiceActionsCell';

const DASH = '—';

type Variant = 'success' | 'destructive' | 'secondary' | 'warning' | 'outline' | 'neutral';

function activeVariant(state?: string): Variant {
  switch (state) {
    case 'active':
      return 'success';
    case 'failed':
      return 'destructive';
    case 'activating':
    case 'deactivating':
    case 'reloading':
      return 'warning';
    default:
      return 'secondary';
  }
}

function enabledVariant(state?: string): Variant {
  switch (state) {
    case 'enabled':
    case 'enabled-runtime':
      return 'success';
    case 'masked':
      return 'destructive';
    case 'disabled':
      return 'outline';
    default:
      return 'neutral';
  }
}

type HostServicesColumnsArgs = {
  host: HostTarget;
  busyUnit: string | null;
  onAction: (unit: HostService, action: HostServiceAction) => void;
  onEdit: (unit: HostService) => void;
};

export function useHostServicesColumns({
  host,
  busyUnit,
  onAction,
  onEdit,
}: HostServicesColumnsArgs): ColumnDef<HostService, unknown>[] {
  const { t } = useTranslation('environments');

  return useMemo<ColumnDef<HostService, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (unit) => unit.name,
        header: t('hostServices.column.unit'),
        cell: ({ row }) => (
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-foreground">{row.original.name}</span>
            {row.original.protected && (
              <IconShieldLock
                className="size-3.5 text-muted-foreground"
                aria-label={t(`hostServices.protected.${row.original.protected}`)}
              />
            )}
          </span>
        ),
      },
      {
        id: 'description',
        accessorFn: (unit) => unit.description || DASH,
        header: t('hostServices.column.description'),
        meta: { width: 'xl' } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="text-muted-foreground">{row.original.description || DASH}</span>,
      },
      {
        id: 'active',
        accessorFn: (unit) => `${unit.activeState ?? ''} ${unit.subState ?? ''}`.trim() || DASH,
        header: t('hostServices.column.state'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Badge variant={activeVariant(row.original.activeState)}>
              {row.original.activeState || DASH}
              {row.original.subState ? ` · ${row.original.subState}` : ''}
            </Badge>
            <StatusError
              error={row.original.failureReason}
              title={t('hostServices.failureTitle', {
                unit: row.original.name,
              })}
            />
          </div>
        ),
      },
      {
        id: 'enabled',
        accessorFn: (unit) => unit.unitFileState || DASH,
        header: t('hostServices.column.enabled'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.unitFileState ? (
            <Badge variant={enabledVariant(row.original.unitFileState)}>{row.original.unitFileState}</Badge>
          ) : (
            <span className="text-muted-foreground">{DASH}</span>
          ),
      },
      {
        id: 'pid',
        accessorFn: (unit) => (unit.mainPid ? String(unit.mainPid) : DASH),
        header: t('hostServices.column.pid'),
        meta: { width: 'xs' } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="font-mono text-muted-foreground">{row.original.mainPid || DASH}</span>,
      },
      {
        id: 'load',
        accessorFn: (unit) => unit.loadState || DASH,
        header: t('hostServices.column.load'),
        meta: { width: 'sm', defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="text-muted-foreground">{row.original.loadState || DASH}</span>,
      },
      {
        id: 'actions',
        header: t('labels.actions', { ns: 'common' }),
        enableSorting: false,
        meta: {
          width: 'lg',
          locked: true,
          noExport: true,
          label: t('labels.actions', { ns: 'common' }),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <HostServiceActionsCell
            host={host}
            unit={row.original}
            busy={busyUnit === row.original.name}
            onAction={onAction}
            onEdit={onEdit}
          />
        ),
      },
    ],
    [t, host, busyUnit, onAction, onEdit],
  );
}
