// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The reading half of fleet-health. FleetHealthWidget draws; this decides.
//
// Splitting it is not ceremony: the widget answers four separate questions
// (how is the fleet split, what is the one-line verdict, what are the three
// supporting facts, what wants a human) and three of them need data the
// container list alone does not carry. Keeping the derivation here leaves
// the component as markup, which is the only way it stays legible.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconAlertTriangle,
  IconArchiveOff,
  IconArrowUp,
  IconCircleCheck,
  IconPlayerStop,
  IconRefreshAlert,
} from '@tabler/icons-react';
import { useContainers } from '@modules/containers/hooks/useContainers';
import { useContainerUpdateResults } from '@modules/containers/hooks/useContainerUpdates';
import { useAllContainerBackupPlans } from '@resources/hooks/useContainerBackups';
import { parseHealth, parseUptime } from '@modules/containers/components/container-utils';
import { useEnvironmentStore } from '@resources/stores/environment';
import {
  SNAPSHOT_OVERDUE_MS,
  ageSince,
  buildSegments,
  containerName,
  type AttentionItem,
  type FleetHealth,
} from './fleetHealthModel';

export function useFleetHealth(): FleetHealth {
  const { t } = useTranslation('dashboard');
  const envId = useEnvironmentStore((state) => state.currentId);
  const { data: containers = [], isLoading, isError } = useContainers();
  const { data: plans } = useAllContainerBackupPlans(envId);
  const updateResults = useContainerUpdateResults();

  return useMemo(() => {
    let running = 0;
    let unhealthy = 0;
    let paused = 0;
    let exited = 0;
    let restarting = 0;
    let dead = 0;

    const items: AttentionItem[] = [];

    for (const container of containers) {
      const label = containerName(container.Names, container.Id);
      const health = parseHealth(container.Status);

      if (container.State === 'running') running += 1;

      if (health === 'unhealthy') {
        unhealthy += 1;
        const uptime = parseUptime(container.Status);
        items.push({
          id: `unhealthy:${container.Id}`,
          rank: 0,
          icon: IconAlertTriangle,
          tone: 'warning',
          title: t('fleetHealthWidget.cards.unhealthy', { name: label }),
          detail:
            uptime === '-'
              ? t('fleetHealthWidget.cards.unhealthyDetailPlain')
              : t('fleetHealthWidget.cards.unhealthyDetail', { uptime }),
          to: `/containers/${container.Id}`,
        });
        continue;
      }

      if (container.State === 'dead') {
        dead += 1;
        exited += 1;
        items.push({
          id: `dead:${container.Id}`,
          rank: 0,
          icon: IconAlertTriangle,
          tone: 'danger',
          title: t('fleetHealthWidget.cards.dead', { name: label }),
          detail: t('fleetHealthWidget.cards.deadDetail'),
          to: `/containers/${container.Id}`,
        });
        continue;
      }

      if (container.State === 'restarting') {
        restarting += 1;
        items.push({
          id: `restarting:${container.Id}`,
          rank: 0,
          icon: IconRefreshAlert,
          tone: 'danger',
          title: t('fleetHealthWidget.cards.restarting', { name: label }),
          detail: t('fleetHealthWidget.cards.restartingDetail'),
          to: `/containers/${container.Id}`,
        });
        continue;
      }

      if (container.State === 'paused') paused += 1;
      else if (container.State === 'exited') exited += 1;
    }

    const overdue = plans
      .filter((plan) => plan.enabled)
      .map((plan) => ({
        name: plan.containerName || plan.name,
        age: ageSince(plan.lastRunAt),
      }))
      .filter((plan) => plan.age === null || plan.age > SNAPSHOT_OVERDUE_MS)
      // Never-run first, then the stalest.
      .sort((a, b) => (b.age ?? Infinity) - (a.age ?? Infinity));

    if (overdue.length > 0) {
      items.push({
        id: 'snapshots-overdue',
        rank: 1,
        icon: IconArchiveOff,
        tone: 'danger',
        title: t('fleetHealthWidget.cards.snapshots', { count: overdue.length }),
        detail: overdue
          .slice(0, 2)
          .map((plan) =>
            plan.age === null
              ? `${plan.name} (${t('fleetHealthWidget.cards.never')})`
              : `${plan.name} (${t('fleetHealthWidget.cards.days', {
                  count: Math.max(Math.floor(plan.age / 86_400_000), 1),
                })})`,
          )
          .join(' · '),
        to: '/backups',
      });
    }

    // Exited containers are one aggregate card — a list of six stopped
    // containers is a list, and the list already has a widget.
    const stopped = exited - dead;
    if (stopped > 0) {
      items.push({
        id: 'exited',
        rank: 2,
        icon: IconPlayerStop,
        tone: 'neutral',
        title: t('fleetHealthWidget.cards.exited', { count: stopped }),
        detail: t('fleetHealthWidget.cards.exitedDetail'),
        to: '/containers?state=exited',
      });
    }

    const updatesReady = updateResults
      ? [...updateResults.values()].filter((result) => result.updateAvailable).length
      : 0;

    if (updatesReady > 0) {
      items.push({
        id: 'updates',
        rank: 3,
        icon: IconArrowUp,
        tone: 'info',
        title: t('fleetHealthWidget.cards.updates', { count: updatesReady }),
        detail: t('fleetHealthWidget.cards.updatesDetail'),
        to: '/containers',
      });
    }

    items.sort((a, b) => a.rank - b.rank);

    if (items.length === 0 && containers.length > 0) {
      items.push({
        id: 'all-clear',
        rank: 9,
        icon: IconCircleCheck,
        tone: 'success',
        title: t('fleetHealthWidget.cards.allClear'),
        detail: t('fleetHealthWidget.cards.allClearDetail'),
        to: '/containers',
      });
    }

    const segments = buildSegments(running, unhealthy, paused, exited);
    const facts = [
      t('fleetHealthWidget.facts.running', {
        running,
        total: containers.length,
      }),
      updateResults
        ? updatesReady > 0
          ? t('fleetHealthWidget.facts.updates', { count: updatesReady })
          : t('fleetHealthWidget.facts.noUpdates')
        : t('fleetHealthWidget.facts.updatesUnchecked'),
      restarting > 0
        ? t('fleetHealthWidget.facts.restarting', { count: restarting })
        : t('fleetHealthWidget.facts.noRestarts'),
    ];

    return {
      isLoading,
      isError,
      total: containers.length,
      running,
      segments,
      down: dead + restarting,
      facts,
      items,
    };
  }, [containers, plans, updateResults, isLoading, isError, t]);
}
