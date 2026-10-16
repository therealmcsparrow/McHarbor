// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// swarm-overview — nodes, services, tasks and stacks in four cells, with a
// verdict on top. Off-swarm environments say so instead of showing zeros.

import { useTranslation } from 'react-i18next';
import { IconServerBolt, IconServerOff } from '@tabler/icons-react';
import {
  WidgetBadge,
  WidgetEmpty,
  WidgetFrame,
  WidgetLoading,
  WidgetTile,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import { useSwarmOverview } from '../hooks/useSwarmCluster';
import { useWidgetContext } from './mount';

export const PLUGIN_PATH = '/plugins/addon-swarm';

export function SwarmOverviewWidget() {
  const { t } = useTranslation('swarm');
  const { navigate } = useWidgetContext();
  const capability = useSwarmCapability();
  const overview = useSwarmOverview(capability.isManager);
  const go = (path: string) => () => navigate(`${PLUGIN_PATH}/${path}`);

  if (capability.isLoading || (capability.isManager && overview.isLoading)) {
    return (
      <WidgetFrame title={t('widgets.swarmOverview.title')}>
        <WidgetLoading label={t('widgets.common.loading')} />
      </WidgetFrame>
    );
  }
  if (!capability.isSwarm || !capability.isManager || !overview.data) {
    return (
      <WidgetFrame title={t('widgets.swarmOverview.title')}>
        <WidgetEmpty
          icon={<IconServerOff className="size-5" aria-hidden="true" />}
          title={!capability.isSwarm ? t('widgets.common.notSwarm') : t('widgets.common.notManager')}
          hint={t('widgets.common.openHint')}
        />
      </WidgetFrame>
    );
  }

  const o = overview.data;
  const problems = o.nodesDown + o.servicesDegraded + o.tasksFailed;
  const tone: Tone = o.nodesDown > 0 ? 'danger' : problems > 0 ? 'warning' : 'success';
  const verdict =
    o.nodesDown > 0
      ? t('widgets.swarmOverview.nodesDown', { count: o.nodesDown })
      : o.servicesDegraded > 0
        ? t('widgets.swarmOverview.servicesDegraded', { count: o.servicesDegraded })
        : o.tasksFailed > 0
          ? t('widgets.swarmOverview.tasksFailed', { count: o.tasksFailed })
          : t('widgets.swarmOverview.allConverged');

  return (
    <WidgetFrame
      title={t('widgets.swarmOverview.title')}
      badge={o.cluster?.name ? <WidgetBadge>{o.cluster.name}</WidgetBadge> : undefined}
      action={
        <span className={tone === 'success' ? 'text-success' : tone === 'warning' ? 'text-warning' : 'text-danger'}>
          <IconServerBolt className="size-4" aria-hidden="true" />
        </span>
      }
    >
      <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-3">
        <p className="shrink-0 text-base leading-snug font-semibold text-foreground">{verdict}</p>
        <div className="grid min-h-0 flex-1 grid-cols-2 gap-2 sm:grid-cols-4">
          <WidgetTile
            label={t('widgets.swarmOverview.nodes')}
            value={`${o.nodesReady}/${o.nodesTotal}`}
            hint={t('widgets.swarmOverview.managersWorkers', { managers: o.managers, workers: o.workers })}
            tone={o.nodesDown > 0 ? 'danger' : o.nodesDraining > 0 ? 'warning' : 'success'}
            share={o.nodesTotal > 0 ? o.nodesReady / o.nodesTotal : 0}
            onClick={go('nodes')}
          />
          <WidgetTile
            label={t('widgets.swarmOverview.services')}
            value={`${o.servicesConverged}/${o.servicesTotal}`}
            hint={t('widgets.swarmOverview.degraded', { count: o.servicesDegraded })}
            tone={o.servicesDegraded > 0 ? 'warning' : 'success'}
            share={o.servicesTotal > 0 ? o.servicesConverged / o.servicesTotal : 0}
            onClick={go('services')}
          />
          <WidgetTile
            label={t('widgets.swarmOverview.tasks')}
            value={`${o.tasksRunning}/${o.tasksDesired}`}
            hint={t('widgets.swarmOverview.failed', { count: o.tasksFailed })}
            tone={o.tasksFailed > 0 ? 'danger' : o.tasksRunning < o.tasksDesired ? 'warning' : 'success'}
            share={o.tasksDesired > 0 ? o.tasksRunning / o.tasksDesired : 0}
            onClick={go('tasks')}
          />
          <WidgetTile
            label={t('widgets.swarmOverview.stacks')}
            value={o.stacks}
            hint={t('widgets.swarmOverview.secretsConfigs', { secrets: o.secrets, configs: o.configs })}
            tone="neutral"
            onClick={go('stacks')}
          />
        </div>
      </div>
    </WidgetFrame>
  );
}
