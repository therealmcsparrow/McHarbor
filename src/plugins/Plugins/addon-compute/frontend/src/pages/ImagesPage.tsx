// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFileUpload, IconRefresh } from '@tabler/icons-react';
import { TablePage } from '@resources/layout/TablePage';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Select } from '@resources/components/ui/Select';
import { COMPUTE_PERM } from '../permissions';
import {
  useCapability,
  useEvictImage,
  useFetchImage,
  useEnvironments,
  useFetchImageEverywhere,
  useReloadCatalog,
} from '../hooks/useCompute';
import { useHostEnvironment } from '../hooks/useHostEnvironment';
import { useComputeView } from '../hooks/useComputeView';
import { useImageCatalogView, type KindFacet } from '../hooks/useImageCatalogView';
import { CardGrid } from '../components/CardGrid';
import { ImageCard } from '../components/ImageCard';
import { useImageColumns } from '../components/ImageColumns';
import { ViewToggle } from '../components/ViewToggle';
import { ImageDialogs } from '../components/ImageDialogs';
import { UnavailableNotice } from '../components/CapabilityPanel';
import type { ComputeImage } from '../types';

const KINDS: readonly KindFacet[] = ['all', 'cloud_image', 'rootfs', 'iso'];

/**
 * The image catalog.
 *
 * Downloads go straight from the distribution to the chosen host — nothing
 * passes through McHarbor — so a row's state is per host, and the whole page is
 * scoped to the host's own topbar selection.
 */
type ImagesPageProps = { uploadRequested?: boolean; onUploadRequestDone?: () => void };
export function ImagesPage({ uploadRequested = false, onUploadRequestDone }: ImagesPageProps) {
  const { t } = useTranslation('compute');

  // The host's topbar selection, and nothing else. This page used to carry its
  // own picker, which quietly overrode the global one.
  const environmentId = useHostEnvironment();
  const { viewMode, setViewMode } = useComputeView('images');
  const {
    isLoading,
    isFetching,
    isError,
    refetch,
    query,
    setQuery,
    searchMode,
    setSearchMode,
    kind,
    setKind,
    distro,
    setDistro,
    operatingSystems,
    byDistro,
    images,
    searchFields,
  } = useImageCatalogView(environmentId);

  const [unverified, setUnverified] = useState<ComputeImage | null>(null);
  const [evicting, setEvicting] = useState<ComputeImage | null>(null);
  const [uploading, setUploading] = useState(false);
  const reloadCatalog = useReloadCatalog();
  const fetchEverywhere = useFetchImageEverywhere();
  const { data: environments } = useEnvironments();

  const { data: host } = useCapability(environmentId || undefined);
  const fetchImage = useFetchImage();
  const evict = useEvictImage();

  // A download runs on the host, not here, so a host that cannot take one says
  // so before the click rather than after it. Otherwise every Download on the
  // page fails one at a time, each with the same host-level reason.
  const hostUsable = !environmentId || host?.available !== false;
  const canDownload = Boolean(environmentId) && hostUsable;

  const download = useCallback(
    (image: ComputeImage) => {
      // An image with no published checksum needs an explicit acknowledgement,
      // and the backend records it on the audit row. Asking here rather than
      // letting the request fail keeps the decision where the operator is.
      if (!image.checksum && !image.checksumUrl) {
        setUnverified(image);
        return;
      }
      fetchImage.mutate({ imageId: image.id, environmentId });
    },
    [fetchImage, environmentId],
  );

  const columns = useImageColumns({
    canDownload,
    downloading: fetchImage.isPending || fetchEverywhere.isPending,
    onDownload: download,
    onDownloadEverywhere: (image) => fetchEverywhere.mutate(image.id),
    onEvict: setEvicting,
  });

  // Named rather than counted: "2 hosts share a library" is what tells the
  // operator a second download would be the same bytes twice.
  const sharedHosts = (environments ?? []).filter((env) => env.sharedLibrary);

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  return (
    <>
      <TablePage
        id="compute-images"
        title={t('images.title')}
        description={
          sharedHosts.length > 0
            ? `${t('images.subtitle')} ${t('images.sharedLibraryNote', {
                count: sharedHosts.length,
                hosts: sharedHosts.map((env) => env.name || env.environmentId).join(', '),
              })}`
            : t('images.subtitle')
        }
        banner={
          hostUsable ? undefined : <UnavailableNotice reason={host?.unavailableReason} required="3.0.0" current="" />
        }
        search={{
          value: query,
          onChange: setQuery,
          mode: searchMode,
          onModeChange: setSearchMode,
          placeholder: t('images.searchPlaceholder'),
          fields: searchFields,
        }}
        filters={
          <div className="w-56">
            <Select
              value={distro}
              ariaLabel={t('images.filter.os')}
              options={[{ value: 'all', label: t('images.filter.allOs') }, ...operatingSystems]}
              onChange={setDistro}
            />
          </div>
        }
        facets={{
          items: KINDS.map((id) => ({
            id,
            label: t(`images.kind.${id}`),
            count: id === 'all' ? byDistro.length : byDistro.filter((image) => image.kind === id).length,
          })),
          value: kind,
          onChange: (next) => setKind(next as KindFacet),
        }}
        actions={<ViewToggle viewMode={viewMode} setViewMode={setViewMode} />}
        toolbar={{
          actions: (
            <>
              <Button
                size="sm"
                variant="outline"
                permission={COMPUTE_PERM.MANAGE}
                disabled={reloadCatalog.isPending}
                onClick={() => reloadCatalog.mutate()}
              >
                <IconRefresh className="h-4 w-4" aria-hidden="true" />
                {t('images.catalog.reload')}
              </Button>
              <Button size="sm" permission={COMPUTE_PERM.IMAGES_UPLOAD} onClick={() => setUploading(true)}>
                <IconFileUpload className="h-4 w-4" aria-hidden="true" />
                {t('images.upload.action')}
              </Button>
            </>
          ),
          columns: viewMode === 'table',
          density: viewMode === 'table',
          refresh: { onRefresh: () => void refetch(), isFetching },
          export: { filename: 'mcharbor-compute-images' },
        }}
        columns={columns}
        data={images}
        loading={isLoading}
        empty={{ title: t('images.empty.title'), description: t('images.empty.body') }}
        getRowId={(image) => image.id}
        body={
          viewMode === 'table'
            ? undefined
            : (pageImages) => (
                <div className="min-h-0 flex-1 overflow-auto" data-scroll-exempt>
                  <CardGrid
                    count={pageImages.length}
                    isLoading={isLoading}
                    filtered={images.length > 0}
                    emptyLabel={t('images.empty.title')}
                    columns={5}
                  >
                    {pageImages.map((image) => (
                      <ImageCard
                        key={image.id}
                        image={image}
                        canDownload={canDownload}
                        downloading={fetchImage.isPending}
                        onDownload={download}
                        onEvict={setEvicting}
                      />
                    ))}
                  </CardGrid>
                </div>
              )
        }
      />

      <ImageDialogs
        environmentId={environmentId}
        uploading={uploading || uploadRequested}
        onCloseUpload={() => {
          setUploading(false);
          onUploadRequestDone?.();
        }}
        unverified={unverified}
        onCloseUnverified={() => setUnverified(null)}
        evicting={evicting}
        onCloseEvict={() => setEvicting(null)}
        fetchImage={fetchImage}
        evict={evict}
      />
    </>
  );
}
