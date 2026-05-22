// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconLayoutGrid, IconLayoutList } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { DataGrid } from '@resources/components/DataGrid';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { PageHeader } from '@resources/layout/PageHeader';
import type { ImageInfo } from '@core/types/docker';
import {
  useExportImage,
  useImages,
  useImportImage,
  usePullImage,
  usePruneImages,
  useRemoveImage,
} from '../hooks/useImages';
import { useImagesPageConfig } from '../hooks/useImagesPageConfig';
import { useImagesViewStore } from '../stores/images-view';
import { ImageCardGrid } from '../components/ImageCardGrid';
import { ImageImportDialog } from '../components/ImageImportDialog';

export default function ImagesPage() {
  const { t } = useTranslation('images');
  const navigate = useNavigate();
  const { data: images = [], isLoading } = useImages();
  const pullImage = usePullImage();
  const removeImage = useRemoveImage();
  const pruneImages = usePruneImages();
  const importImage = useImportImage();
  const { exportImage } = useExportImage();
  const { viewMode, setViewMode } = useImagesViewStore();
  const [importOpen, setImportOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  function handleExport(image: ImageInfo) {
    const name = image.RepoTags?.[0] ?? image.Id?.replace('sha256:', '').slice(0, 12) ?? 'image';
    exportImage(image.Id, name);
  }

  const { columns, batchActions } = useImagesPageConfig({
    onExport: handleExport,
    onRemove: setConfirmTarget,
    onBatchRemove: (rows) => {
      for (const row of rows) {
        removeImage.mutate(row.Id);
      }
    },
    onPrune: () => pruneImages.mutate(),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description', { count: images.length })}
        actions={
          <>
            <Button onClick={() => setImportOpen(true)}>
              <IconDownload className="h-4 w-4" /> {t('import.title')}
            </Button>
            <div className="h-6 w-px bg-border" />
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
          </>
        }
      />

      {viewMode === 'table' ? (
        <DataGrid
          data={images}
          columns={columns}
          searchKey="Id"
          searchPlaceholder={t('searchPlaceholder')}
          loading={isLoading}
          emptyMessage={t('emptyMessage')}
          onRowClick={(image) => navigate(`/images/${encodeURIComponent(image.Id)}`)}
          selectable
          batchActions={batchActions}
          getRowId={(row) => row.Id}
        />
      ) : (
        <ImageCardGrid
          images={images}
          isLoading={isLoading}
          onClick={(image) => navigate(`/images/${encodeURIComponent(image.Id)}`)}
          onRemove={setConfirmTarget}
        />
      )}

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

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('confirm.removeTitle')}
        description={t('confirm.removeDescription')}
        onConfirm={() => {
          if (confirmTarget) {
            removeImage.mutate(confirmTarget);
          }
          setConfirmTarget(null);
        }}
        loading={removeImage.isPending}
      />
    </div>
  );
}
