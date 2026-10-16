// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// NetworkDetailPage — built to the container detail page's grammar.
//
// Before, the whole action header portalled into the topbar and the page
// opened with no title: you had to read the global chrome to know which
// network you were looking at, and that chrome changed shape depending on
// the route. Everything below it was one long scroll — general, IPAM,
// options, labels, then the container table — with nothing to tell you how
// far down it went.
//
// It now reads the way a container reads: breadcrumb in the topbar (with a
// leaf switcher over the other networks), a hero card carrying identity,
// state and actions, the tab rail inside that card, and one concern at a
// time in the panel below. `?tab=` addresses a face directly.

import { useCallback, useMemo } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageBody } from '@resources/layout/PageBody';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { NetworkDetailPageDialogs } from './NetworkDetailPageDialogs';
import { NetworkDetailTabContent } from '../components/NetworkDetailTabContent';
import { NetworkDetailTabs, isNetworkTabId, type NetworkTabId } from '../components/NetworkDetailTabs';
import { isBuiltInNetwork } from '@core/utils/network';
import { useNetworks } from '../hooks/useNetworks';
import { useNetworkDetailState } from '../hooks/useNetworkDetailState';
import { NetworkDetailHeader } from './NetworkDetailHeader';

export default function NetworkDetailPage() {
  const { id = '' } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { t } = useTranslation('networks');
  const state = useNetworkDetailState(id, t);

  const requestedTab = searchParams.get('tab');
  const activeTab: NetworkTabId = isNetworkTabId(requestedTab) ? requestedTab : 'overview';

  const setActiveTab = useCallback(
    (tab: NetworkTabId) => {
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

  // Peers for the breadcrumb leaf switcher, so moving between two networks
  // does not mean a round trip through the list. The list is already cached
  // by the networks page in the common case.
  const { data: allNetworks = [] } = useNetworks();
  const siblings: BreadcrumbSibling[] = useMemo(
    () =>
      allNetworks.map((peer) => ({
        id: peer.Id,
        label: peer.Name,
        to: `/networks/${peer.Id}`,
      })),
    [allNetworks],
  );

  if (state.isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <div className="grid gap-4 lg:grid-cols-2">
          <CardSkeleton lines={6} />
          <CardSkeleton lines={6} />
        </div>
      </div>
    );
  }

  if (!state.network) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('detail.notFound')}
          primaryAction={{
            label: t('detail.backToNetworks'),
            onClick: () => navigate('/networks'),
          }}
        />
      </div>
    );
  }

  const network = state.network;
  const builtIn = isBuiltInNetwork(network);

  return (
    /* AppLayout strips the shell's `p-6` whenever the header slot is active,
       and `useNetworkDetailState` claims that slot — so the padding comes
       back here, on the page that loses it. */
    <div className="flex h-full min-h-0 flex-col">
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/networks' }, { label: network.Name }]}
        siblings={siblings}
        currentId={network.Id}
      />

      <PageBody className="gap-4 p-6">
        <Card className="flex-none overflow-visible">
          <NetworkDetailHeader
            name={network.Name}
            id={network.Id}
            driver={network.Driver}
            scope={network.Scope}
            builtIn={builtIn}
            connectedCount={state.connectedContainers.length}
            editing={state.editing}
            saving={state.saving}
            onEdit={state.startEditing}
            onSave={() => state.setRecreateDialogOpen(true)}
            onCancelEdit={state.cancelEditing}
            onRemove={() => state.setRemoveDialogOpen(true)}
          />
          <NetworkDetailTabs
            activeTab={activeTab}
            onTabChange={setActiveTab}
            labelFor={(tab) => tabLabel(tab, t)}
            ariaLabel={t('detail.tabs.ariaLabel')}
            tabBadges={{ containers: state.connectedContainers.length }}
          />
        </Card>

        <div
          id="network-detail-panel"
          role="tabpanel"
          aria-labelledby={`network-tab-${activeTab}`}
          tabIndex={0}
          className="flex min-h-0 flex-col"
        >
          <NetworkDetailTabContent
            activeTab={activeTab}
            network={network}
            editData={state.editData}
            editing={state.editing}
            connectedContainers={state.connectedContainers}
            t={t}
            onUpdateEdit={state.updateEdit}
            onUpdateIPAMEntry={state.updateIPAMEntry}
            onAddIPAMEntry={state.addIPAMEntry}
            onRemoveIPAMEntry={state.removeIPAMEntry}
            onOpenConnect={() => {
              state.setSelectedContainer('');
              state.setConnectDialogOpen(true);
            }}
            onDisconnect={state.setDisconnectTarget}
            onContainerOpen={(containerID) => navigate(`/containers/${containerID}`)}
          />
        </div>
      </PageBody>

      <NetworkDetailPageDialogs network={network} id={id} state={state} t={t} onRemoved={() => navigate('/networks')} />
    </div>
  );
}

/**
 * Tab labels as literal `t('...')` calls rather than an interpolated key —
 * the i18n CI gate reads literals, and a template key is invisible to it.
 */
function tabLabel(tab: NetworkTabId, t: (key: string) => string): string {
  switch (tab) {
    case 'overview':
      return t('detail.tabs.overview');
    case 'containers':
      return t('detail.tabs.containers');
    case 'configuration':
      return t('detail.tabs.configuration');
  }
}
