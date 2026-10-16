// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// swarm-nodes — membership: role, availability and state per node, with the
// unhealthy ones first.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCrown, IconServer2, IconServerOff } from '@tabler/icons-react';
import {
  IconTile,
  ROW_HEIGHT,
  ToneDot,
  WidgetBadge,
  WidgetEmpty,
  WidgetFooterLink,
  WidgetFrame,
  WidgetList,
  WidgetLoading,
  WidgetRow,
  useFittedRows,
  type Tone,
} from '@modules/dashboard/components/widget-kit';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import { useSwarmNodes } from '../hooks/useSwarmNodes';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH } from './SwarmOverviewWidget';

function nodeTone(state: string, availability: string): Tone {
  if (state === 'down' || state === 'unknown') return 'danger';
  if (state === 'disconnected') return 'danger';
  if (availability === 'drain') return 'warning';
  if (availability === 'pause') return 'warning';
  return 'success';
}

export function SwarmNodesWidget() {
  const { t } = useTranslation('swarm');
  const { navigate } = useWidgetContext();
  const capability = useSwarmCapability();
  const nodes = useSwarmNodes(capability.isManager);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const rows = useMemo(() => {
    return (nodes.data ?? [])
      .map((n) => ({ ...n, tone: nodeTone(n.state, n.availability) }))
      .sort((a, b) => {
        const rank = (tone: Tone) => (tone === 'danger' ? 0 : tone === 'warning' ? 1 : 2);
        return (
          rank(a.tone) - rank(b.tone) || Number(b.isLeader) - Number(a.isLeader) || a.hostname.localeCompare(b.hostname)
        );
      });
  }, [nodes.data]);

  const ready = rows.filter((r) => r.tone === 'success').length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.swarmNodes.title')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge tone={ready === rows.length ? 'success' : 'warning'}>
            {t('widgets.swarmNodes.readyOf', { ready, total: rows.length })}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink label={t('widgets.swarmNodes.viewAll')} onClick={() => navigate(`${PLUGIN_PATH}/nodes`)} />
        </>
      }
    >
      {capability.isLoading || (capability.isManager && nodes.isLoading) ? (
        <WidgetLoading label={t('widgets.common.loading')} />
      ) : !capability.isManager ? (
        <WidgetEmpty
          icon={<IconServerOff className="size-5" aria-hidden="true" />}
          title={!capability.isSwarm ? t('widgets.common.notSwarm') : t('widgets.common.notManager')}
        />
      ) : rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconServer2 className="size-5" aria-hidden="true" />}
          title={t('widgets.swarmNodes.none')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((n) => (
            <li key={n.id}>
              <WidgetRow
                leading={
                  <IconTile tone={n.tone}>
                    {n.isLeader ? (
                      <IconCrown className="size-4" aria-hidden="true" />
                    ) : (
                      <IconServer2 className="size-4" aria-hidden="true" />
                    )}
                  </IconTile>
                }
                title={n.hostname}
                subtitle={`${t(`widgets.swarmNodes.role.${n.role}`)} · ${n.addr}${n.taskCount > 0 ? ` · ${t('widgets.swarmNodes.tasks', { count: n.taskCount })}` : ''}`}
                trailing={
                  <>
                    <span className="capitalize">{n.availability === 'active' ? n.state : n.availability}</span>
                    <ToneDot tone={n.tone} />
                  </>
                }
                onClick={() => navigate(`${PLUGIN_PATH}/nodes/${n.id}`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
