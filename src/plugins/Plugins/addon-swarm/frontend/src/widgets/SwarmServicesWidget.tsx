// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// swarm-services — every service with its task convergence, degraded first.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconServerOff, IconStack2 } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';
import {
  ROW_HEIGHT,
  TONE_SOLID,
  TONE_TEXT,
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
import { useSwarmServices } from '../hooks/useSwarmServices';
import { useWidgetContext } from './mount';
import { PLUGIN_PATH } from './SwarmOverviewWidget';

export function SwarmServicesWidget() {
  const { t } = useTranslation('swarm');
  const { navigate } = useWidgetContext();
  const capability = useSwarmCapability();
  const services = useSwarmServices(capability.isManager);
  const { ref, capacity } = useFittedRows(ROW_HEIGHT);

  const rows = useMemo(() => {
    return (services.data ?? [])
      .map((s) => {
        const desired = s.mode === 'global' ? Math.max(s.desiredTasks, s.runningTasks) : s.desiredTasks;
        const share = desired > 0 ? Math.min(1, s.runningTasks / desired) : 1;
        const updating = s.updateState === 'updating' || s.updateState === 'rollback_started';
        const tone: Tone =
          s.updateState === 'paused' || s.updateState === 'rollback_paused'
            ? 'danger'
            : updating
              ? 'info'
              : desired > 0 && s.runningTasks === 0
                ? 'danger'
                : share < 1
                  ? 'warning'
                  : 'success';
        return {
          id: s.id,
          name: s.name,
          stack: s.stackName,
          mode: s.mode,
          running: s.runningTasks,
          desired,
          share,
          tone,
          updateState: s.updateState,
        };
      })
      .sort((a, b) => a.share - b.share || a.name.localeCompare(b.name));
  }, [services.data]);

  const degraded = rows.filter((r) => r.tone === 'warning' || r.tone === 'danger').length;
  const shown = rows.slice(0, capacity);
  const remaining = rows.length - shown.length;

  return (
    <WidgetFrame
      title={t('widgets.swarmServices.title')}
      badge={
        rows.length > 0 ? (
          <WidgetBadge tone={degraded > 0 ? 'warning' : 'success'}>
            {degraded > 0
              ? t('widgets.swarmServices.degraded', { count: degraded })
              : t('widgets.swarmServices.allConverged')}
          </WidgetBadge>
        ) : undefined
      }
      footer={
        <>
          <span>{remaining > 0 ? t('widgets.common.more', { count: remaining }) : null}</span>
          <WidgetFooterLink
            label={t('widgets.swarmServices.viewAll')}
            onClick={() => navigate(`${PLUGIN_PATH}/services`)}
          />
        </>
      }
    >
      {capability.isLoading || (capability.isManager && services.isLoading) ? (
        <WidgetLoading label={t('widgets.common.loading')} />
      ) : !capability.isManager ? (
        <WidgetEmpty
          icon={<IconServerOff className="size-5" aria-hidden="true" />}
          title={!capability.isSwarm ? t('widgets.common.notSwarm') : t('widgets.common.notManager')}
        />
      ) : rows.length === 0 ? (
        <WidgetEmpty
          icon={<IconStack2 className="size-5" aria-hidden="true" />}
          title={t('widgets.swarmServices.none')}
        />
      ) : (
        <WidgetList listRef={ref}>
          {shown.map((row) => (
            <li key={row.id}>
              <WidgetRow
                title={row.name}
                subtitle={row.stack ? `${row.stack} · ${row.mode}` : row.mode}
                trailing={
                  <>
                    <span className="h-1.5 w-16 overflow-hidden rounded-full bg-glass-2" aria-hidden="true">
                      <span
                        className={cn('block h-full w-(--w) rounded-full', TONE_SOLID[row.tone])}
                        // The fill is the convergence itself; no utility class says "3 of 5 tasks".
                        style={cssVars({ w: `${Math.round(row.share * 100)}%` })}
                      />
                    </span>
                    <span
                      className={cn(
                        'w-10 text-right font-medium tabular-nums',
                        row.tone === 'success' ? 'text-foreground' : TONE_TEXT[row.tone],
                      )}
                    >
                      {row.running}/{row.desired}
                    </span>
                  </>
                }
                onClick={() => navigate(`${PLUGIN_PATH}/services/${row.id}`)}
              />
            </li>
          ))}
        </WidgetList>
      )}
    </WidgetFrame>
  );
}
