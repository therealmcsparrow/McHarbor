// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconFilterOff, IconLayoutGrid, IconLayoutList } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { TablePage } from '@resources/layout/TablePage';
import { createSearchMatcher, matchesSearchFields, type SearchMode } from '@resources/utils/search-filter';
import type { ImageInfo } from '@core/types/docker';
import { isProtectedImage } from '@core/utils/protection';
import { useExportImage, useImages, useImportImage, usePullImage, useRemoveImage } from '@resources/hooks/useImages';
import { useImagesPageConfig } from '../hooks/useImagesPageConfig';
import { useImagesViewStore } from '../stores/images-view';
import { ImageCardGrid } from '../components/ImageCardGrid';
import { ImageImportDialog } from '../components/ImageImportDialog';
import { PruneImagesDialog } from '../components/PruneImagesDialog';
import { RemoveImageDialog } from '../components/RemoveImageDialog';
import { PERM } from '@core/auth/permissionKeys';

export default function ImagesPage() {
  const { t } = useTranslation('images');
  const navigate = useNavigate();
  const { data: images = [], isLoading, isFetching, refetch } = useImages();
  const pullImage = usePullImage();
  const removeImage = useRemoveImage();
  const importImage = useImportImage();
  const { exportImage } = useExportImage();
  const { viewMode, setViewMode } = useImagesViewStore();
  const [importOpen, setImportOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);
  const [pruneConfirmOpen, setPruneConfirmOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [searchMode, setSearchMode] = useState<SearchMode>('contains');

  function handleExport(image: ImageInfo) {
    const exportName = image.RepoTags?.[0] ?? image.Id?.replace('sha256:', '').slice(0, 12) ?? 'image';
    exportImage(image.Id, exportName);
  }

  // The card view gets the same query, the same match modes and the same
  // fields as the table — it used to have a separate search box with
  // contains-only matching over a different pair of fields.
  const matcher = useMemo(() => createSearchMatcher(query, searchMode), [query, searchMode]);
  const cardImages = useMemo(
    () =>
      query.trim() === ''
        ? images
        : images.filter(
            (image) => matchesSearchFields(query, searchMode, [image.Id, ...(image.RepoTags ?? [])]).matched,
          ),
    [images, query, searchMode],
  );

  const { columns, batchActions } = useImagesPageConfig({
    onExport: handleExport,
    onRemove: setConfirmTarget,
    onBatchRemove: (rows) => {
      for (const row of rows) {
        if (isProtectedImage(row)) continue;
        removeImage.mutate(row.Id);
      }
    },
  });

  return (
    <>
      <TablePage
        id="images"
        title={t('title')}
        description={t('description', { count: images.length })}
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
          fields: (image) => [image.Id, ...(image.RepoTags ?? [])],
        }}
        toolbar={{
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-images' },
          // The card view used to hand-roll its own copy of these two buttons
          // next to a second search box. Both views share this one set now.
          actions: (
            <>
              <Button permission={PERM.IMAGES_MANAGE} onClick={() => setImportOpen(true)}>
                <IconDownload className="h-4 w-4" /> {t('import.title')}
              </Button>
              <Button permission={PERM.IMAGES_MANAGE} variant="outline" onClick={() => setPruneConfirmOpen(true)}>
                <IconFilterOff className="h-4 w-4" /> {t('pruneUnused')}
              </Button>
            </>
          ),
        }}
        columns={columns}
        data={images}
        loading={isLoading}
        empty={{ title: t('emptyMessage') }}
        selectable
        batchActions={batchActions}
        getRowId={(row) => row.Id}
        onRowClick={(image) => navigate(`/images/${encodeURIComponent(image.Id)}`)}
        body={
          viewMode === 'table' ? undefined : (
            <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
              <ImageCardGrid
                images={cardImages}
                isLoading={isLoading}
                onClick={(image) => navigate(`/images/${encodeURIComponent(image.Id)}`)}
                onRemove={setConfirmTarget}
              />
            </div>
          )
        }
      />

      <ImageImportDialog
        open={importOpen}
        pullPending={pullImage.isPending}
        importPending={importImage.isPending}
        onOpenChange={setImportOpen}
        onPull={(imageName) =>
          pullImage.mutate(imageName, {
            onSuccess: () => setImportOpen(false),
          })
        }
        onImport={(file) =>
          importImage.mutate(file, {
            onSuccess: () => setImportOpen(false),
          })
        }
      />

      <RemoveImageDialog
        imageId={confirmTarget}
        blocked={images.some((item) => item.Id === confirmTarget && isProtectedImage(item))}
        onOpenChange={(next) => !next && setConfirmTarget(null)}
      />

      <PruneImagesDialog open={pruneConfirmOpen} onOpenChange={setPruneConfirmOpen} />
    </>
  );
}
