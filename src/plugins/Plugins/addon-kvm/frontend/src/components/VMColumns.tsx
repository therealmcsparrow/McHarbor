// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { formatVmMemory } from './vm-resources';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { VMStateBadge } from './VMStateBadge';
import { GuestAddress, VMActions, type VMAction } from './VMActions';
import { formatRelative } from '../format';
import type { VM } from '../types';

interface UseVMColumnsOptions {
  hostName: (environmentId: string) => string;
  busy: boolean;
  diagnosePending: boolean;
  onOpen: (id: string) => void;
  onAction: (id: string, action: VMAction) => void;
  onDelete: (vm: VM) => void;
  onDiagnose: (vm: VM) => void;
}

/** Zero-pads IPv4 octets so a string compare orders addresses numerically; null when there is no address. */
function addressSortKey(address: string | undefined): string | null {
  if (!address) return null;
  const octets = address.split('.');
  if (octets.length !== 4 || octets.some((o) => !/^\d{1,3}$/.test(o))) return address;
  return octets.map((o) => o.padStart(3, '0')).join('.');
}

/** The guest list's columns. */
export function useVMColumns({
  hostName,
  busy,
  diagnosePending,
  onOpen,
  onAction,
  onDelete,
  onDiagnose,
}: UseVMColumnsOptions) {
  const { t } = useTranslation('vm');

  return useMemo<ColumnDef<VM, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('list.columns.name'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <PlainButton
              onClick={() => onOpen(row.original.id)}
              className="text-left font-medium text-foreground hover:text-primary hover:underline"
            >
              {row.original.name}
            </PlainButton>
            {row.original.isTemplate && <Badge variant="secondary">{t('nav.templates')}</Badge>}
          </div>
        ),
      },
      {
        accessorKey: 'state',
        header: t('list.columns.state'),
        cell: ({ row }) => <VMStateBadge vm={row.original} />,
      },
      {
        id: 'host',
        header: t('list.columns.host'),
        accessorFn: (vm) => hostName(vm.environmentId),
        cell: ({ row }) => <span className="text-muted-foreground">{hostName(row.original.environmentId)}</span>,
      },
      {
        id: 'resources',
        header: t('list.columns.resources'),
        meta: { sortValue: (vm: VM) => vm.vcpus * 1_000_000 + vm.memoryMb },
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-1">
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {t('resources.cpu', { count: row.original.vcpus })}
            </span>
            <span className="rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {t('resources.ram', { size: formatVmMemory(row.original.memoryMb) })}
            </span>
          </div>
        ),
      },
      {
        id: 'address',
        header: t('list.columns.address'),
        meta: { sortValue: (vm: VM) => addressSortKey(vm.guestReportedIps?.[0]) },
        cell: ({ row }) => <GuestAddress vm={row.original} />,
      },
      {
        id: 'uptime',
        header: t('list.columns.uptime'),
        meta: {
          sortValue: (vm: VM) => (vm.state === 'running' && vm.lastStartedAt ? Date.parse(vm.lastStartedAt) : null),
        },
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {row.original.state === 'running' && row.original.lastStartedAt
              ? formatRelative(row.original.lastStartedAt)
              : '—'}
          </span>
        ),
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex justify-end gap-1">
            <VMActions
              vm={row.original}
              busy={busy}
              diagnosePending={diagnosePending}
              onOpen={onOpen}
              onAction={(action) => onAction(row.original.id, action)}
              onDelete={onDelete}
              onDiagnose={onDiagnose}
            />
          </div>
        ),
      },
    ],
    [t, hostName, busy, diagnosePending, onOpen, onAction, onDelete, onDiagnose],
  );
}
