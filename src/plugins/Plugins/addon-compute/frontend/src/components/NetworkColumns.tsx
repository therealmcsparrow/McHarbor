// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconAddressBook, IconPencil, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { attachmentLabel, hostLinkLabel } from '../utils/networkAttachment';
import { COMPUTE_PERM } from '../permissions';
import type { Network } from '../types';

interface UseNetworkColumnsOptions {
  hostName: (environmentId: string) => string;
  onViewLeases: (network: Network) => void;
  onEdit: (network: Network) => void;
  onDelete: (network: Network) => void;
}

/** The networks table's columns. */
export function useNetworkColumns({ hostName, onViewLeases, onEdit, onDelete }: UseNetworkColumnsOptions) {
  const { t } = useTranslation('compute');

  return useMemo<ColumnDef<Network, unknown>[]>(
    () => [
      {
        accessorKey: 'name',
        header: t('networks.columns.name'),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="font-medium text-foreground">{row.original.name}</span>
            {row.original.isDefault && <Badge variant="secondary">{t('badges.default')}</Badge>}
          </div>
        ),
      },
      {
        id: 'host',
        header: t('networks.columns.host'),
        accessorFn: (network) => hostName(network.environmentId),
        cell: ({ row }) => <span className="text-muted-foreground">{hostName(row.original.environmentId)}</span>,
      },
      {
        accessorKey: 'mode',
        header: t('networks.columns.mode'),
        cell: ({ row }) => (
          <Badge variant={row.original.mode === 'managed' ? 'default' : 'outline'}>
            {t(`networks.mode.${row.original.mode}`)}
          </Badge>
        ),
      },
      {
        accessorKey: 'bridge',
        header: t('networks.columns.attachment'),
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{attachmentLabel(row.original)}</code>
        ),
      },
      {
        id: 'hostInterface',
        header: t('networks.columns.hostInterface'),
        accessorFn: (network) => hostLinkLabel(network),
        cell: ({ row }) => (
          <code className="font-mono text-xs text-muted-foreground">{hostLinkLabel(row.original) || '—'}</code>
        ),
      },
      {
        accessorKey: 'subnetCidr',
        header: t('networks.columns.subnet'),
        cell: ({ row }) => <span className="text-muted-foreground">{row.original.subnetCidr || '—'}</span>,
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        meta: { locked: true, noExport: true, label: t('labels.actions', { ns: 'common' }) },
        enableSorting: false,
        cell: ({ row }) => (
          <div className="flex justify-end gap-1">
            {/*
              Leases only exist on a managed network. A bridged or direct one
              gets its addresses from the LAN's own DHCP server, which McHarbor
              does not see.
            */}
            {row.original.mode === 'managed' && (
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.viewLeases')}
                title={t('actions.viewLeases')}
                permission={COMPUTE_PERM.VIEW}
                onClick={() => onViewLeases(row.original)}
              >
                <IconAddressBook className="h-4 w-4" aria-hidden="true" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.edit')}
              title={t('actions.edit')}
              permission={COMPUTE_PERM.MANAGE}
              onClick={() => onEdit(row.original)}
            >
              <IconPencil className="h-4 w-4" aria-hidden="true" />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.delete')}
              title={t('actions.delete')}
              permission={COMPUTE_PERM.DELETE}
              onClick={() => onDelete(row.original)}
            >
              <IconTrash className="h-4 w-4 text-destructive" aria-hidden="true" />
            </Button>
          </div>
        ),
      },
    ],
    [t, hostName, onViewLeases, onEdit, onDelete],
  );
}
