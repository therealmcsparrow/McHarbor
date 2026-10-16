// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useLayoutEffect, useMemo, useState } from 'react';
import { usePluginDetailId, usePluginNavigate, usePluginSearchParams } from '../nav-context';
import { useTranslation } from 'react-i18next';
import { IconHome } from '@tabler/icons-react';
import { Card } from '@resources/components/ui/Card';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { PluginBreadcrumb, type PluginSibling } from '../components/PluginBreadcrumb';
import { DetailTabs } from '../components/DetailTabs';
import { ScaleServiceDialog } from '../components/ScaleServiceDialog';
import { SwarmGuard } from '../components/SwarmGuard';
import { SwarmTaskTable } from '../components/SwarmTaskTable';
import { useRemoveSwarmService, useSwarmService, useSwarmServices } from '../hooks/useSwarmServices';
import { ServiceDetailHeader } from '../components/ServiceDetailHeader';
import { OverviewPanel, ConfigPanel, LabelsPanel, LogsPanel } from '../components/ServiceDetailPanels';

// SwarmServiceDetailPage — built to the network detail page's grammar.
//
// Breadcrumb in the topbar with a leaf switcher over the other services, a
// hero card carrying identity, state and actions, the tab rail inside that
// card, and one concern at a time in the panel below. `?tab=` addresses a
// face directly so a link can point at the logs rather than at the page.

const TABS = ['overview', 'tasks', 'logs', 'config', 'labels'] as const;
type TabId = (typeof TABS)[number];

function isTabId(value: string | null): value is TabId {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function SwarmServiceDetailPage() {
  return (
    <SwarmGuard>
      <ServiceDetail />
    </SwarmGuard>
  );
}

function ServiceDetail() {
  const id = usePluginDetailId();
  const navigate = usePluginNavigate();
  const { t } = useTranslation('swarm');
  const [searchParams, setSearchParams] = usePluginSearchParams();

  const { data: service, isLoading, error } = useSwarmService(id);
  const { data: allServices = [] } = useSwarmServices();

  const remove = useRemoveSwarmService();
  const [scaleOpen, setScaleOpen] = useState(false);
  const [removeOpen, setRemoveOpen] = useState(false);

  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  const requestedTab = searchParams.get('tab');
  const activeTab: TabId = isTabId(requestedTab) ? requestedTab : 'overview';

  const setActiveTab = useCallback(
    (tab: TabId) => {
      setSearchParams(
        (current) => {
          if (tab === 'overview') current.delete('tab');
          else current.set('tab', tab);
          return current;
        },
        { preventScrollReset: true },
      );
    },
    [setSearchParams],
  );

  const siblings: PluginSibling[] = useMemo(
    () =>
      allServices.map((peer) => ({
        id: peer.id,
        label: peer.name,
        to: `services/${peer.id}`,
      })),
    [allServices],
  );

  if (isLoading) {
    return (
      <div className="flex h-full w-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={6} />
        <CardSkeleton lines={6} />
      </div>
    );
  }

  if (error || !service) {
    return (
      <div className="w-full">
        <ErrorState
          variant="page"
          title={t('detail.serviceNotFound')}
          error={error}
          primaryAction={{
            label: t('detail.backToServices'),
            onClick: () => navigate('services'),
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col gap-4">
      <PluginBreadcrumb
        crumbs={[
          { to: '/dashboard', label: <IconHome className="size-3" /> },
          { label: t('services.title'), to: 'services' },
          { label: service.name },
        ]}
        siblings={siblings}
        currentId={service.id}
      />

      <ServiceDetailHeader service={service} onScale={() => setScaleOpen(true)} onRemove={() => setRemoveOpen(true)} />

      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <DetailTabs
            ariaLabel={t('detail.tabsLabel')}
            value={activeTab}
            onChange={setActiveTab}
            tabs={TABS.map((tab) => ({ id: tab, label: t(`detail.tabs.${tab}`) }))}
          />
        </div>
      </Card>

      <div className="min-h-0 flex-1 overflow-auto">
        {activeTab === 'overview' && <OverviewPanel service={service} />}
        {activeTab === 'tasks' && (
          <SwarmTaskTable
            id="swarm-service-tasks"
            header={false}
            title={t('tasks.title')}
            filter={{ service: service.id }}
            hide={{ service: true }}
          />
        )}
        {activeTab === 'logs' && <LogsPanel id={service.id} />}
        {activeTab === 'config' && <ConfigPanel service={service} />}
        {activeTab === 'labels' && <LabelsPanel service={service} />}
      </div>

      <ScaleServiceDialog service={scaleOpen ? service : null} onOpenChange={(nextOpen) => setScaleOpen(nextOpen)} />

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={t('dialogs.removeService.title')}
        description={t('dialogs.removeService.description', { name: service.name })}
        loading={remove.isPending}
        onConfirm={() => remove.mutate(service.id, { onSuccess: () => navigate('services') })}
      />
    </div>
  );
}
