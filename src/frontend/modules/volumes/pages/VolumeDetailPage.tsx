// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// VolumeDetailPage — new, built to the container detail page's grammar.
//
// Volumes were the one resource with no detail page: the list showed a name,
// a driver and a mountpoint, and that was the whole story. Which containers
// mount it, at what path, read-only or not — the question you actually have
// when deciding whether a volume is safe to prune — had no answer anywhere in
// the UI.
//
// The backend already served `GET /volumes/{name}`; nothing was asking. This
// page asks, and lays the answer out the way every other detail page does:
// breadcrumb in the topbar with a leaf switcher, hero card carrying identity
// and state, the tab rail inside it, one concern at a time below.
//
// The container list is the source of truth for "what mounts this" — Docker's
// volume inspect does not carry a back-reference, so the relationship is
// derived from each container's `Mounts`.

import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageBody } from '@resources/layout/PageBody';
import { useContainers } from '@resources/hooks/useContainers';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useVolume, useVolumes } from '../hooks/useVolumes';
import { RemoveVolumeDialog } from '../components/RemoveVolumeDialog';
import { useVolumeDetailData } from './useVolumeDetailData';
import { VolumeDetailTabContent } from '../components/VolumeDetailTabContent';
import { VolumeDetailTabs, volumeTabIDs, type VolumeTabId } from '../components/VolumeDetailTabs';
import { VolumeDetailHeader } from './VolumeDetailHeader';

export default function VolumeDetailPage() {
  const { name: volumeParam = '' } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('volumes');
  const { data: volume, isLoading } = useVolume(volumeParam);
  const { data: allVolumes = [] } = useVolumes();
  const { data: containers = [] } = useContainers();
  const [removeOpen, setRemoveOpen] = useState(false);
  const [activeTab, setActiveTab] = useTabFromUrl<VolumeTabId>(volumeTabIDs, 'overview', {
    storageKey: `mcharbor-tabs:volume:${volumeParam || 'unknown'}`,
  });

  // Claiming the header slot puts the breadcrumb in the topbar and hands the
  // shell's padding back to this page.
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  const { mounts, siblings } = useVolumeDetailData(volumeParam, containers, allVolumes);

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <div className="grid gap-4 lg:grid-cols-2">
          <CardSkeleton lines={5} />
          <CardSkeleton lines={5} />
        </div>
      </div>
    );
  }

  if (!volume) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('detail.notFound')}
          primaryAction={{
            label: t('detail.backToVolumes'),
            onClick: () => navigate('/volumes'),
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/volumes' }, { label: volume.Name }]}
        siblings={siblings}
        currentId={volume.Name}
      />

      <PageBody className="gap-4 p-6">
        <Card className="flex-none overflow-visible">
          <VolumeDetailHeader volume={volume} mountedCount={mounts.length} onRemove={() => setRemoveOpen(true)} />
          <VolumeDetailTabs
            activeTab={activeTab}
            onTabChange={setActiveTab}
            labelFor={(tab) => tabLabel(tab, t)}
            ariaLabel={t('detail.tabs.ariaLabel')}
            tabBadges={{ containers: mounts.length }}
          />
        </Card>

        <div
          id="volume-detail-panel"
          role="tabpanel"
          aria-labelledby={`volume-tab-${activeTab}`}
          tabIndex={0}
          className="flex min-h-0 flex-col"
        >
          <VolumeDetailTabContent
            activeTab={activeTab}
            volume={volume}
            mounts={mounts}
            t={t}
            onContainerOpen={(containerID) => navigate(`/containers/${containerID}`)}
          />
        </div>
      </PageBody>

      <RemoveVolumeDialog
        volumeName={removeOpen ? volume.Name : null}
        onOpenChange={(next) => !next && setRemoveOpen(false)}
        onSuccess={() => navigate('/volumes')}
      />
    </div>
  );
}

/**
 * Tab labels as literal `t('...')` calls rather than an interpolated key —
 * the i18n CI gate reads literals, and a template key is invisible to it.
 */
function tabLabel(tab: VolumeTabId, t: (key: string) => string): string {
  switch (tab) {
    case 'overview':
      return t('detail.tabs.overview');
    case 'containers':
      return t('detail.tabs.containers');
    case 'configuration':
      return t('detail.tabs.configuration');
  }
}
