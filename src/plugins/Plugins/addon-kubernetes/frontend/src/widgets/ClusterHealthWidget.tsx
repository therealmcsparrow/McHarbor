// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// cluster-health — "is the cluster fine?" in one tile.
//
// A verdict line, one proportional bar of pod phases, and four cells that
// each land on the page that explains them.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCloudOff, IconHexagons } from '@tabler/icons-react';
import {
  SegmentBar,
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetLoading,
  WidgetTile,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { list } from '../format';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH, podBuckets, readyParts, useSummaryWidget } from './useSummaryWidget';

export function ClusterHealthWidget() {
  const { t } = useTranslation('kubernetes');
  const { navigate } = useWidgetContext();
  const { data: summary, isLoading, isError } = useSummaryWidget();
  const go = (path: string) => () => navigate(`${PLUGIN_PATH}/${path}`);

  if (isLoading) {
    return (
      <WidgetFrame title={t('widgets.clusterHealth.title')}>
        <WidgetLoading label={t('widgets.common.loading')} />
      </WidgetFrame>
    );
  }
  if (isError || !summary || !summary.connected) {
    const reason = summary?.reason;
    return (
      <WidgetFrame title={t('widgets.clusterHealth.title')}>
        <WidgetEmpty
          tone={reason === 'unreachable' ? 'danger' : 'neutral'}
          icon={<IconCloudOff className="size-5" aria-hidden="true" />}
          title={reason === 'unreachable' ? t('widgets.common.unreachable') : t('widgets.common.notConfigured')}
          hint={t('widgets.common.openHint')}
        />
      </WidgetFrame>
    );
  }

  const pods = podBuckets(summary.pods);
  const totalPods = summary.pods.length;
  const unreadyNodes = summary.nodes.filter((node) => list(node.status) !== 'Ready').length;
  const stalled = summary.deployments.filter((row) => {
    const { ready, total } = readyParts(row);
    return total > 0 && ready !== total;
  }).length;
  const warnings = summary.events.filter((evt) => list(evt.type) === 'Warning').length;

  const problems = pods.failed + unreadyNodes + stalled;
  const verdictTone: Tone =
    pods.failed > 0 || unreadyNodes > 0 ? 'danger' : stalled > 0 || pods.pending > 0 ? 'warning' : 'success';
  const verdict =
    pods.failed > 0
      ? t('widgets.clusterHealth.failedPods', { count: pods.failed })
      : unreadyNodes > 0
        ? t('widgets.clusterHealth.unreadyNodes', { count: unreadyNodes })
        : stalled > 0
          ? t('widgets.clusterHealth.stalledRollouts', { count: stalled })
          : pods.pending > 0
            ? t('widgets.clusterHealth.pendingPods', { count: pods.pending })
            : t('widgets.clusterHealth.allHealthy');

  return (
    <WidgetFrame
      title={t('widgets.clusterHealth.title')}
      badge={summary.version ? <WidgetBadge>{summary.version}</WidgetBadge> : undefined}
      action={
        <span
          className={
            verdictTone === 'success' ? 'text-success' : verdictTone === 'warning' ? 'text-warning' : 'text-danger'
          }
        >
          {problems > 0 ? (
            <IconAlertTriangle className="size-4" aria-hidden="true" />
          ) : (
            <IconHexagons className="size-4" aria-hidden="true" />
          )}
        </span>
      }
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3 px-4 pb-3">
        <p className="shrink-0 text-base leading-snug font-semibold text-foreground">{verdict}</p>
        <SegmentBar
          segments={[
            {
              key: 'running',
              label: t('widgets.clusterHealth.phase.running'),
              count: pods.running,
              tone: 'success',
              onClick: go('pods'),
            },
            {
              key: 'pending',
              label: t('widgets.clusterHealth.phase.pending'),
              count: pods.pending,
              tone: 'warning',
              onClick: go('pods'),
            },
            {
              key: 'failed',
              label: t('widgets.clusterHealth.phase.failed'),
              count: pods.failed,
              tone: 'danger',
              onClick: go('pods'),
            },
            {
              key: 'succeeded',
              label: t('widgets.clusterHealth.phase.succeeded'),
              count: pods.succeeded,
              tone: 'info',
              onClick: go('pods'),
            },
            {
              key: 'other',
              label: t('widgets.clusterHealth.phase.other'),
              count: pods.other,
              tone: 'neutral',
              onClick: go('pods'),
            },
          ]}
        />
        <div className="grid min-h-0 flex-1 grid-cols-2 gap-2 sm:grid-cols-4">
          <WidgetTile
            label={t('tabs.pods')}
            value={totalPods}
            hint={t('widgets.clusterHealth.inNamespaces', { count: summary.namespaces.length })}
            tone={pods.failed > 0 ? 'danger' : 'neutral'}
            onClick={go('pods')}
          />
          <WidgetTile
            label={t('tabs.deployments')}
            value={summary.deployments.length - stalled}
            hint={t('widgets.clusterHealth.ofReady', { total: summary.deployments.length })}
            tone={stalled > 0 ? 'warning' : 'success'}
            share={
              summary.deployments.length > 0 ? (summary.deployments.length - stalled) / summary.deployments.length : 0
            }
            onClick={go('pods')}
          />
          <WidgetTile
            label={t('tabs.nodes')}
            value={summary.nodes.length - unreadyNodes}
            hint={t('widgets.clusterHealth.ofReady', { total: summary.nodes.length })}
            tone={unreadyNodes > 0 ? 'danger' : 'success'}
            share={summary.nodes.length > 0 ? (summary.nodes.length - unreadyNodes) / summary.nodes.length : 0}
            onClick={go('nodes')}
          />
          <WidgetTile
            label={t('widgets.clusterHealth.warnings')}
            value={warnings}
            hint={t('widgets.clusterHealth.recentEvents', { count: summary.events.length })}
            tone={warnings > 0 ? 'warning' : 'neutral'}
            onClick={go('nodes')}
          />
        </div>
      </div>
    </WidgetFrame>
  );
}
