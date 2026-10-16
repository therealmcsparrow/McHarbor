// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ImageDetailPage — built to the container detail page's grammar.
//
// Before, this was four cards in one grid with the header portalled into the
// topbar: the page opened with no title, and a forty-row layer history shared
// a screen with the eight-line summary you actually came for.
//
// Now: breadcrumb in the topbar (with a leaf switcher over the other images),
// a hero card carrying identity, state and actions, the tab rail inside that
// card, and one of the four cards at a time in the panel below.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageBody } from '@resources/layout/PageBody';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { truncateId } from '@resources/utils/format';
import { useImage, useImages, useImageHistory, useExportImage } from '@resources/hooks/useImages';
import { ImageConfigCard } from '../components/ImageConfigCard';
import { RemoveImageDialog } from '../components/RemoveImageDialog';
import { ImageDetailHeader } from '../components/ImageDetailHeader';
import { ImageDetailTabs, imageTabIDs, type ImageTabId } from '../components/ImageDetailTabs';
import { ImageHistoryCard } from '../components/ImageHistoryCard';
import { ImageLabelsCard } from '../components/ImageLabelsCard';
import { ImageOverviewCard } from '../components/ImageOverviewCard';

export default function ImageDetailPage() {
  const { t } = useTranslation('images');
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: image, isLoading } = useImage(id ?? '');
  const { data: layerHistory = [] } = useImageHistory(id ?? '');
  const { data: allImages = [] } = useImages();
  const { exportImage } = useExportImage();
  const [removeOpen, setRemoveOpen] = useState(false);
  const [activeTab, setActiveTab] = useTabFromUrl<ImageTabId>(imageTabIDs, 'overview', {
    storageKey: `mcharbor-tabs:image:${id ?? 'unknown'}`,
  });

  // The page claims the header slot for its breadcrumb, which also tells
  // AppLayout to hand the shell's padding back to the page.
  const setHeaderActive = useHeaderSlot((state) => state.setActive);
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Peers for the breadcrumb leaf switcher, so moving between two images does
  // not mean a round trip through the list.
  const siblings: BreadcrumbSibling[] = useMemo(
    () =>
      allImages.map((peer) => ({
        id: peer.Id,
        label: peer.RepoTags?.[0] ?? truncateId(peer.Id.replace('sha256:', '')),
        to: `/images/${peer.Id}`,
      })),
    [allImages],
  );

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </div>
    );
  }

  if (!image) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('notFound')}
          primaryAction={{
            label: t('detail.back'),
            onClick: () => navigate('/images'),
          }}
        />
      </div>
    );
  }

  const imageName = image.RepoTags?.[0] ?? truncateId(image.Id.replace('sha256:', ''));

  return (
    <div className="flex h-full min-h-0 flex-col">
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/images' }, { label: imageName }]}
        siblings={siblings}
        currentId={image.Id}
      />

      <PageBody className="gap-4 p-6">
        <Card className="flex-none overflow-visible">
          <ImageDetailHeader
            image={image}
            onExport={() => exportImage(image.Id, image.RepoTags?.[0] ?? image.Id.replace('sha256:', '').slice(0, 12))}
            onRemove={() => setRemoveOpen(true)}
          />
          <ImageDetailTabs
            activeTab={activeTab}
            onTabChange={setActiveTab}
            labelFor={(tab) => tabLabel(tab, t)}
            ariaLabel={t('detail.tabs.ariaLabel')}
            tabBadges={{
              labels: Object.keys(image.Config?.Labels ?? {}).length,
              history: layerHistory.length,
            }}
          />
        </Card>

        <div
          id="image-detail-panel"
          role="tabpanel"
          aria-labelledby={`image-tab-${activeTab}`}
          tabIndex={0}
          className="flex min-h-0 flex-col"
        >
          {activeTab === 'overview' && <ImageOverviewCard image={image} />}
          {activeTab === 'config' && <ImageConfigCard image={image} />}
          {activeTab === 'labels' && <ImageLabelsCard image={image} />}
          {activeTab === 'history' && <ImageHistoryCard imageId={image.Id} history={layerHistory} />}
        </div>
      </PageBody>

      <RemoveImageDialog
        imageId={removeOpen ? image.Id : null}
        onOpenChange={(next) => !next && setRemoveOpen(false)}
        onSuccess={() => navigate('/images')}
      />
    </div>
  );
}

/**
 * Tab labels as literal `t('...')` calls rather than an interpolated key —
 * the i18n CI gate reads literals, and a template key is invisible to it.
 */
function tabLabel(tab: ImageTabId, t: (key: string) => string): string {
  switch (tab) {
    case 'overview':
      return t('detail.tabs.overview');
    case 'config':
      return t('detail.tabs.config');
    case 'labels':
      return t('detail.tabs.labels');
    case 'history':
      return t('detail.tabs.history');
  }
}
