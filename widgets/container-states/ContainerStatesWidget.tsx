// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { IconBoxOff } from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';
import { WidgetEmpty, WidgetFrame, WidgetLoading, type Tone } from '@modules/dashboard/components/widget-kit';
import { DonutPanel } from '@modules/dashboard/components/widget-donut';

const STATE_TONE: Record<string, Tone> = {
  running: 'success',
  exited: 'danger',
  stopped: 'neutral',
  paused: 'warning',
  restarting: 'warning',
  created: 'info',
  dead: 'critical',
};

/** Order the legend by consequence, not by count. */
const STATE_ORDER = ['running', 'restarting', 'paused', 'created', 'exited', 'stopped', 'dead'];

export default function ContainerStatesWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const navigate = useNavigate();
  const { data: containers, isLoading } = useContainers(true);

  const segments = useMemo(() => {
    const counts: Record<string, number> = {};
    for (const c of containers ?? []) {
      const state = c.State ?? 'unknown';
      counts[state] = (counts[state] ?? 0) + 1;
    }
    return Object.entries(counts)
      .filter(([, value]) => value > 0)
      .sort(([a], [b]) => {
        const ia = STATE_ORDER.indexOf(a);
        const ib = STATE_ORDER.indexOf(b);
        return (ia === -1 ? 99 : ia) - (ib === -1 ? 99 : ib);
      })
      .map(([stateName, value]) => ({
        key: stateName,
        label: stateName.charAt(0).toUpperCase() + stateName.slice(1),
        value,
        tone: STATE_TONE[stateName] ?? 'neutral',
        onClick: () => navigate(`/containers?state=${stateName}`),
      }));
  }, [containers, navigate]);

  const total = segments.reduce((sum, s) => sum + s.value, 0);

  return (
    <WidgetFrame title={t('containerStatesWidget.title')}>
      {isLoading ? (
        <WidgetLoading label={t('loading')} />
      ) : total === 0 ? (
        <WidgetEmpty
          icon={<IconBoxOff className="size-5" aria-hidden="true" />}
          title={t('containerStatesWidget.noContainers')}
        />
      ) : (
        <DonutPanel segments={segments} centerValue={total} centerLabel={t('containerStatesWidget.total')} />
      )}
    </WidgetFrame>
  );
}
