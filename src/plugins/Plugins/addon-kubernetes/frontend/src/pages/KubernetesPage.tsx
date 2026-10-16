// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// KubernetesPage — the cluster browser.
//
// Ported from `src/frontend/modules/kubernetes/pages/KubernetesPage.tsx`. Two
// things changed in the move, and only two:
//
//   1. Routing. The core page read `useLocation()` and called `useNavigate()`.
//      This bundle carries its own react-router copy with no Router context,
//      so both would throw and blank the plugin root — the position comes from
//      the host's `PluginNavContext.splat` instead, and navigation goes back
//      through the host's own router.
//   2. The cluster. `useEnvironmentStore.kubernetesId` was a host store field
//      for a host concept; the plugin owns its clusters now, so the selection
//      lives in the plugin's own store and the request says `?cluster=`.
//
// The layout is unchanged from the restyle that immediately preceded this
// migration: no `PageLayout` (the shell already pads and scrolls), the shared
// `SubNav` rail, tables flush on the page background, cards for the
// document-shaped tabs.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  IconBox,
  IconChartBar,
  IconDatabase,
  IconInfoCircle,
  IconListDetails,
  IconServer,
  IconSettings,
  IconTopologyStar3,
  IconWorld,
} from '@tabler/icons-react';
import type { TablerIcon } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { PageHeader } from '@resources/layout/PageHeader';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { cn } from '@resources/utils/cn';
import { ClusterHeader } from '../components/ClusterHeader';
import { KubernetesActionDialogs, useKubernetesActionDialogs } from '../components/KubernetesActionDialogs';
import type { ActionsKind } from '../components/KubernetesActionsCell';
import { KubernetesDetail } from '../components/KubernetesDetail';
import { KubernetesOverview, KubernetesResources } from '../components/KubernetesOverview';
import { KubernetesTabs } from '../components/KubernetesTabs';
import { KubernetesTables, type TableId } from '../components/KubernetesTables';
import { clusterParams, useSelectedCluster } from '../cluster-store';
import { Disconnected, Loading } from './ClusterStates';
import { resourcePath } from '../format';
import { kubernetesGroupOf, kubernetesTabs, type TabId } from '../nav';
import { usePluginNavigate, usePluginSplat } from '../nav-context';
import { useTabMenu } from '../hooks/useTabMenu';
import type { KubernetesSummary, Row } from '../types';

const tabs: TabId[] = kubernetesTabs;
const icons: Record<TabId, TablerIcon> = {
  overview: IconInfoCircle,
  resources: IconChartBar,
  pods: IconBox,
  deployments: IconTopologyStar3,
  workloads: IconTopologyStar3,
  services: IconServer,
  networking: IconWorld,
  namespaces: IconListDetails,
  nodes: IconServer,
  events: IconListDetails,
  config: IconSettings,
  storage: IconDatabase,
  capabilities: IconInfoCircle,
};

/** Tabs that read as a document rather than a table, and so scroll the page. */
const DOCUMENT_TABS: TabId[] = ['overview', 'resources'];

export function KubernetesPage() {
  const { t } = useTranslation('kubernetes');
  const clusterId = useSelectedCluster();
  const navigate = usePluginNavigate();
  const splat = usePluginSplat();

  const activeTab = pathTab(splat);
  const detail = detailPath(splat);
  const params = clusterParams(clusterId);

  const setActiveTab = useCallback(
    (tab: TabId) => navigate(resourcePath(tab === 'overview' ? '' : `/${tab}`)),
    [navigate],
  );

  // The sidebar owns the coarse division, so the rail only carries the tabs of
  // the group the reader is standing in. A one-tab group (Storage, Config) has
  // nothing to switch between and renders no rail at all.
  const group = kubernetesGroupOf(activeTab);
  const groupTabs = group.tabs;
  const tabCommands = useMemo(
    () => groupTabs.map((tab) => ({ id: tab, label: t(`tabs.${tab}`), icon: icons[tab] })),
    [groupTabs, t],
  );

  const summary = useQuery({
    queryKey: ['kubernetes', 'summary', clusterId],
    queryFn: () => api.get<KubernetesSummary>('/kubernetes/summary', params).then(assertSuccess),
    refetchInterval: 15_000,
  });
  const detailQuery = useQuery({
    queryKey: ['kubernetes', detail?.kind, detail?.namespace, detail?.name, clusterId],
    enabled: Boolean(detail),
    queryFn: () =>
      api.get<Row>(`/kubernetes/${detail?.kind}/${detail?.namespace}/${detail?.name}`, params).then(assertSuccess),
  });

  // Deleting the resource a detail page is about leaves that page with no
  // subject, so the delete returns the reader to the list it came from — the
  // same move the container detail page makes.
  const onDeleted = useCallback(() => {
    if (detail) navigate(resourcePath(`/${detail.kind}`));
  }, [detail, navigate]);
  const { actions, dialogProps } = useKubernetesActionDialogs(summary.data, onDeleted);
  const refetch = useCallback(() => void summary.refetch(), [summary]);
  const tabContextMenuItems = useTabMenu(activeTab, setActiveTab, refetch);

  // A table fills the frame and lets its own grid scroll — rule 2 of the
  // PageShell contract. Everything else is a column of cards and scrolls here.
  const showsTable =
    Boolean(summary.data?.connected) &&
    !detail &&
    !DOCUMENT_TABS.includes(activeTab) &&
    !summary.isLoading &&
    !summary.error;

  // A detail view brings its own hero — breadcrumb, name, state, actions — so
  // the cluster header and the group's tab rail step aside for it, exactly as
  // the containers list header steps aside for a container detail page.
  const showsChrome = !detail;

  return (
    <div className="flex h-full min-h-0 w-full flex-col gap-4">
      {/* The topbar keeps the title only. Everything specific to the cluster —
          which one, what state it is in, the picker, the refresh — belongs to
          the details card below, not to the global chrome. */}
      {showsChrome && <PageHeader title={t(`groups.${group.id}`)} />}

      {showsChrome && (
        <ClusterHeader
          summary={summary.data}
          onRefresh={refetch}
          isFetching={summary.isFetching}
          showRefresh={!showsTable}
          tabs={
            groupTabs.length > 1 ? (
              <KubernetesTabs
                tabs={tabCommands}
                activeTab={activeTab}
                onTabChange={setActiveTab}
                contextMenuItems={tabContextMenuItems}
              />
            ) : undefined
          }
        />
      )}

      <div
        id="kubernetes-panel"
        role="tabpanel"
        aria-labelledby={showsChrome && groupTabs.length > 1 ? `kubernetes-tab-${activeTab}` : undefined}
        className={cn('flex min-h-0 min-w-0 flex-1 flex-col', !showsTable && 'overflow-y-auto')}
      >
        {summary.isLoading && <Loading />}

        {summary.error && (
          <ErrorState variant="card" title={t('errors.summaryFailed')} error={summary.error} onRetry={refetch} />
        )}

        {summary.data && !summary.data.connected && <Disconnected summary={summary.data} />}

        {summary.data?.connected && detail && (
          <KubernetesDetail
            kind={detail.kind}
            namespace={detail.namespace}
            name={detail.name}
            data={detailQuery.data}
            loading={detailQuery.isLoading}
            actions={{
              onScale: () => actions.onScale(detail),
              onRestart: () => actions.onRestart(detail),
              onDelete: () => actions.onDelete(detail, detail.kind),
              onLogs: () => actions.onLogs(detail),
            }}
          />
        )}

        {summary.data?.connected && !detail && activeTab === 'overview' && (
          <KubernetesOverview summary={summary.data} />
        )}

        {summary.data?.connected && !detail && activeTab === 'resources' && (
          <KubernetesResources summary={summary.data} />
        )}

        {showsTable && summary.data && (
          <KubernetesTables
            summary={summary.data}
            activeTab={activeTab as TableId}
            actions={actions}
            onRefresh={refetch}
            isFetching={summary.isFetching}
          />
        )}
      </div>

      <KubernetesActionDialogs {...dialogProps} />
    </div>
  );
}

function pathTab(splat: string): TabId {
  const first = splat.split('/')[0] as TabId;
  return tabs.includes(first) ? first : 'overview';
}

/**
 * The pod or deployment this URL addresses, or `null` for a list view.
 *
 * The return type names `ActionsKind` rather than leaving `kind` as `string`:
 * the detail hero picks its icon, its health rule and its action set from it,
 * and every one of those is a closed choice between two values.
 */
function detailPath(splat: string): { kind: ActionsKind; namespace: string; name: string } | null {
  const [kind, namespace, itemName] = splat.split('/');
  if ((kind === 'pods' || kind === 'deployments') && namespace && itemName) {
    return { kind, namespace, name: itemName };
  }
  return null;
}
