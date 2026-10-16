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
import { DetailPanel, DetailRow, DetailTabs, KeyValueList } from '../components/DetailTabs';
import { NodeLabelsDialog } from '../components/NodeLabelsDialog';
import { SwarmGuard } from '../components/SwarmGuard';
import { SwarmTaskTable } from '../components/SwarmTaskTable';
import { useRemoveSwarmNode, useSwarmNode, useSwarmNodes } from '../hooks/useSwarmNodes';
import { NodeDetailHeader } from '../components/NodeDetailHeader';
import { NodeOverview } from '../components/NodeOverview';

// SwarmNodeDetailPage — the service detail page's grammar, applied to a node.

const TABS = ['overview', 'tasks', 'labels'] as const;
type TabId = (typeof TABS)[number];

function isTabId(value: string | null): value is TabId {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function SwarmNodeDetailPage() {
  return (
    <SwarmGuard>
      <NodeDetail />
    </SwarmGuard>
  );
}

function NodeDetail() {
  const id = usePluginDetailId();
  const navigate = usePluginNavigate();
  const { t } = useTranslation('swarm');
  const [searchParams, setSearchParams] = usePluginSearchParams();

  const { data: node, isLoading, error } = useSwarmNode(id);
  const { data: allNodes = [] } = useSwarmNodes();
  const remove = useRemoveSwarmNode();

  const [labelsOpen, setLabelsOpen] = useState(false);
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
      allNodes.map((peer) => ({
        id: peer.id,
        label: peer.hostname || peer.id,
        to: `nodes/${peer.id}`,
      })),
    [allNodes],
  );

  if (isLoading) {
    return (
      <div className="flex h-full w-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={6} />
      </div>
    );
  }

  if (error || !node) {
    return (
      <div className="w-full">
        <ErrorState
          variant="page"
          title={t('detail.nodeNotFound')}
          error={error}
          primaryAction={{
            label: t('detail.backToNodes'),
            onClick: () => navigate('nodes'),
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
          { label: t('nodes.title'), to: 'nodes' },
          { label: node.hostname || node.id },
        ]}
        siblings={siblings}
        currentId={node.id}
      />

      <NodeDetailHeader node={node} onEditLabels={() => setLabelsOpen(true)} onRemove={() => setRemoveOpen(true)} />

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
        {activeTab === 'overview' && <NodeOverview node={node} />}
        {activeTab === 'tasks' && (
          <SwarmTaskTable
            id="swarm-node-tasks"
            header={false}
            title={t('tasks.title')}
            filter={{ node: node.id }}
            hide={{ node: true }}
          />
        )}
        {activeTab === 'labels' && (
          <div className="grid gap-4 lg:grid-cols-2">
            <DetailPanel title={t('detail.panels.nodeLabels')}>
              <DetailRow label={t('columns.labels')}>
                <KeyValueList entries={node.labels} />
              </DetailRow>
            </DetailPanel>
            <DetailPanel title={t('detail.panels.engineLabels')}>
              <DetailRow label={t('columns.labels')}>
                <KeyValueList entries={node.engineLabels} />
              </DetailRow>
            </DetailPanel>
          </div>
        )}
      </div>

      <NodeLabelsDialog node={labelsOpen ? node : null} onOpenChange={() => setLabelsOpen(false)} />

      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        title={t('dialogs.removeNode.title')}
        description={t('dialogs.removeNode.description', { name: node.hostname || node.id })}
        loading={remove.isPending}
        onConfirm={() => remove.mutate({ id: node.id }, { onSuccess: () => navigate('nodes') })}
      />
    </div>
  );
}
