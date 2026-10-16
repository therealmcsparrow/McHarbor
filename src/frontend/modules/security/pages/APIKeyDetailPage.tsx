// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// One API key, in full.
//
// Same shape as the user and group detail pages, which it deliberately did
// not have at first: breadcrumb in the topbar, a hero card carrying identity
// and state with the tab rail inside it, and a bare panel underneath. A key
// answers three unrelated questions — what is this credential, what may it
// do, what has it done — and stacking all three on one scrolling page made
// each of them harder to read than it needed to be.
//
// The page works for a deleted key too. Its record is retained until the
// usage retention window expires, and the history outlives the credential on
// purpose: "what did that key do before we removed it" is the question most
// worth being able to answer.

import { useLayoutEffect } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useAPIKeyDetail } from '../hooks/useAPIKeys';
import { useAPIKeyUsageSeries } from '../hooks/useAPIKeyUsage';
import { APIKeyDetailHero } from '../components/APIKeyDetailHero';
import { APIKeyDetailTabs } from '../components/APIKeyDetailTabs';
import { apiKeyDetailTabIDs, type APIKeyDetailTabId } from '../components/api-key-detail-tabs';
import { APIKeyOverviewTab } from '../components/tabs/APIKeyOverviewTab';
import { APIKeyAccessTab } from '../components/tabs/APIKeyAccessTab';
import { APIKeyUsageTab } from '../components/tabs/APIKeyUsageTab';

export default function APIKeyDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('security');
  const { data: detail, isLoading, isError } = useAPIKeyDetail(id);
  const [activeTab, setActiveTab] = useTabFromUrl<APIKeyDetailTabId>(apiKeyDetailTabIDs, 'overview', {
    storageKey: 'mcharbor-tabs:api-key-detail',
  });
  // The same query the usage tab runs, so the overview's request count is a
  // cache read rather than a second round trip.
  const { data: series } = useAPIKeyUsageSeries(id, '24h');

  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </div>
    );
  }

  // A 403 and a 404 land here together on purpose: telling a caller that a
  // key exists but is not theirs is itself a disclosure.
  if (isError || !detail) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('apiKeys.detail.notFoundTitle')}
          description={t('apiKeys.detail.notFoundDescription')}
          primaryAction={{
            label: t('apiKeys.detail.backToKeys'),
            onClick: () => navigate('/security?tab=api-keys'),
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <HeaderBreadcrumb
        crumbs={[
          { label: t('title'), to: '/security' },
          { label: t('tabs.apiKeys'), to: '/security?tab=api-keys' },
          { label: detail.name },
        ]}
      />

      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <APIKeyDetailHero detail={detail} />
        </div>
        <APIKeyDetailTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          ariaLabel={t('apiKeys.detail.tabsLabel')}
          tabBadges={{
            access: detail.effectivePermissions.length,
            usage: series?.total ?? 0,
          }}
        />
      </Card>

      <div
        id="api-key-detail-panel"
        role="tabpanel"
        aria-labelledby={`api-key-tab-${activeTab}`}
        tabIndex={0}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
      >
        {activeTab === 'overview' && (
          <APIKeyOverviewTab detail={detail} onNavigate={setActiveTab} requestCount={series?.total ?? 0} />
        )}
        {activeTab === 'access' && <APIKeyAccessTab detail={detail} />}
        {activeTab === 'usage' && <APIKeyUsageTab apiKeyId={detail.id} deleted={detail.status === 'deleted'} />}
      </div>
    </div>
  );
}
