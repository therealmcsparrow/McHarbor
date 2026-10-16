// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconPlus, IconLayoutGrid, IconLayoutList, IconFilterOff } from '@tabler/icons-react';
import { isProtectedVolume } from '@core/utils/protection';
import { TablePage } from '@resources/layout/TablePage';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import { Button } from '@resources/components/ui/Button';
import { useVolumes, useCreateVolume, useRemoveVolume } from '../hooks/useVolumes';
import { useVolumesPageConfig } from '../hooks/useVolumesPageConfig';
import { PruneVolumesDialog } from '../components/PruneVolumesDialog';
import { RemoveVolumeDialog } from '../components/RemoveVolumeDialog';
import { useVolumesViewStore } from '../stores/volumes-view';
import { CreateVolumeDialog } from '../components/CreateVolumeDialog';
import { VolumeCardGrid } from '../components/VolumeCardGrid';
import { PERM } from '@core/auth/permissionKeys';

export default function VolumesPage() {
  const navigate = useNavigate();
  const { t } = useTranslation('volumes');
  const { data: volumes = [], isLoading, isFetching, refetch } = useVolumes();
  const createVolume = useCreateVolume();
  const removeVolume = useRemoveVolume();
  const { viewMode, setViewMode } = useVolumesViewStore();
  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);
  // A volume name can contain characters that are not path-safe, so it is
  // encoded on the way into the URL and decoded by `useParams` on the way out.
  const openVolume = useCallback(
    (volumeName: string) => navigate(`/volumes/${encodeURIComponent(volumeName)}`),
    [navigate],
  );
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  const { columns, batchActions } = useVolumesPageConfig(setConfirmTarget, (volumeName) =>
    removeVolume.mutate(volumeName),
  );

  // The card view shares the table's query, match mode and fields; it used to
  // have a separate box that matched Name/Driver with contains only.
  const matcher = useMemo(() => createSearchMatcher(query, searchMode), [query, searchMode]);
  const cardVolumes = useMemo(
    () =>
      query.trim() === ''
        ? volumes
        : volumes.filter(
            (volume) => matchesSearchFields(query, searchMode, [volume.Name, volume.Driver, volume.Mountpoint]).matched,
          ),
    [volumes, query, searchMode],
  );

  return (
    <>
      <TablePage
        id="volumes"
        title={t('title')}
        description={t('description', { count: volumes.length })}
        actions={
          <div className="flex items-center rounded-lg border border-border">
            <Button
              variant={viewMode === 'table' ? 'default' : 'ghost'}
              size="icon-sm"
              onClick={() => setViewMode('table')}
              aria-label={t('tableView')}
            >
              <IconLayoutList className="h-4 w-4" />
            </Button>
            <Button
              variant={viewMode === 'card' ? 'default' : 'ghost'}
              size="icon-sm"
              onClick={() => setViewMode('card')}
              aria-label={t('cardView')}
            >
              <IconLayoutGrid className="h-4 w-4" />
            </Button>
          </div>
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('searchPlaceholder'),
          error: matcher.error !== null,
          fields: (volume) => [volume.Name, volume.Driver, volume.Mountpoint],
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-volumes' },
          // Shared by both views; the card view used to hand-roll its own copy.
          actions: (
            <>
              <Button permission={PERM.VOLUMES_MANAGE} onClick={() => setCreateOpen(true)}>
                <IconPlus className="h-4 w-4" /> {t('create.title')}
              </Button>
              <Button permission={PERM.VOLUMES_MANAGE} variant="outline" onClick={() => setPruneConfirmOpen(true)}>
                <IconFilterOff className="h-4 w-4" /> {t('pruneUnused')}
              </Button>
            </>
          ),
        }}
        columns={columns}
        data={volumes}
        loading={isLoading}
        empty={{ title: t('emptyMessage') }}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.Name}
        onRowClick={(row) => openVolume(row.Name)}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <VolumeCardGrid
                volumes={cardVolumes}
                isLoading={isLoading}
                onRemove={setConfirmTarget}
                onOpen={openVolume}
              />
            </div>
          )
        }
      />

      <CreateVolumeDialog open={createOpen} onOpenChange={setCreateOpen} createVolume={createVolume} />

      <RemoveVolumeDialog
        volumeName={confirmTarget}
        blocked={volumes.some((item) => item.Name === confirmTarget && isProtectedVolume(item))}
        onOpenChange={(next) => !next && setConfirmTarget(null)}
      />

      <PruneVolumesDialog open={pruneConfirmOpen} onOpenChange={setPruneConfirmOpen} />
    </>
  );
}
