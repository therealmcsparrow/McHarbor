// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDeviceDesktop, IconPlus } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { TablePage } from '@resources/layout/TablePage';
import { VM_PERM } from '../permissions';
import { useDisplayProfiles } from '../hooks/useDisplayProfiles';
import type { DisplayProfile } from '../types-display';
import { DisplayProfileDialog } from '../components/DisplayProfileDialog';
import { DisplayProfileRowActions } from '../components/DisplayProfileRowActions';
import { profileDescription, profileName, valueLabel } from '../components/displayProfileLabels';

/** The SPICE display profiles tab: built-ins first, then the operator's own. */
export function DisplayProfilesTable() {
  const { t } = useTranslation('vm');
  const { data, isPending, isFetching, isError, refetch } = useDisplayProfiles();
  // undefined: no dialog; null: creating; a profile: editing it.
  const [editing, setEditing] = useState<DisplayProfile | null | undefined>(undefined);

  const columns = useMemo<ColumnDef<DisplayProfile, unknown>[]>(
    () => [
      {
        id: 'name',
        header: t('displayProfiles.columns.name'),
        accessorFn: (row) => profileName(t, row),
        cell: ({ row }) => (
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate font-medium text-foreground">{profileName(t, row.original)}</span>
              {row.original.builtin && <Badge variant="secondary">{t('displayProfiles.builtinBadge')}</Badge>}
            </div>
            <p className="truncate text-xs text-muted-foreground">{profileDescription(t, row.original)}</p>
          </div>
        ),
      },
      {
        id: 'image',
        header: t('displayProfiles.columns.image'),
        accessorFn: (row) => valueLabel(t, row.imageCompression),
      },
      {
        id: 'streaming',
        header: t('displayProfiles.columns.streaming'),
        accessorFn: (row) => valueLabel(t, row.streamingVideo),
      },
      {
        id: 'jpeg',
        header: t('displayProfiles.columns.jpeg'),
        accessorFn: (row) => valueLabel(t, row.jpegWanCompression),
      },
      {
        id: 'playback',
        header: t('displayProfiles.columns.playback'),
        accessorFn: (row) => valueLabel(t, row.playbackCompression),
      },
      {
        id: 'vms',
        header: t('displayProfiles.columns.vms'),
        accessorFn: (row) => row.vmCount,
      },
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('displayProfiles.columns.actions')}</span>,
        meta: { locked: true, noExport: true, label: t('displayProfiles.columns.actions') },
        enableSorting: false,
        cell: ({ row }) => <DisplayProfileRowActions profile={row.original} onEdit={setEditing} />,
      },
    ],
    [t],
  );

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  return (
    <>
      {editing !== undefined && <DisplayProfileDialog profile={editing} onClose={() => setEditing(undefined)} />}
      <TablePage
        id="kvm-display-profiles"
        title={t('displayProfiles.title')}
        description={t('displayProfiles.description')}
        columns={columns}
        data={data ?? []}
        loading={isPending}
        getRowId={(row) => row.id}
        empty={{
          icon: <IconDeviceDesktop className="h-5 w-5" />,
          title: t('displayProfiles.empty.title'),
          description: t('displayProfiles.empty.body'),
        }}
        toolbar={{
          actions: (
            <Button permission={VM_PERM.MANAGE} onClick={() => setEditing(null)}>
              <IconPlus className="mr-1 size-4" aria-hidden="true" />
              {t('displayProfiles.create')}
            </Button>
          ),
          columns: true,
          density: true,
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'vm-display-profiles' },
        }}
        pagination={{ mode: 'client' }}
      />
    </>
  );
}
