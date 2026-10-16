// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHexagons, IconPlugConnected } from '@tabler/icons-react';
import { PERM } from '../permissions';
import { Button } from '@resources/components/ui/Button';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import { InitClusterDialog, JoinClusterDialog } from './ClusterLifecycleDialogs';

type SwarmGuardProps = {
  children: ReactNode;
  /**
   * When true the page needs the control plane, not merely membership. Every
   * listing in this module does; only the overview can say something useful
   * from a worker.
   */
  requiresManager?: boolean;
};

/**
 * Stands between a Swarm page and an environment that cannot answer it.
 *
 * Three states, and they are genuinely different things:
 *
 *  - still asking — a skeleton, because flashing "not a Swarm" at somebody
 *    whose cluster is fine is worse than a moment of nothing;
 *  - not a Swarm — an empty state with the one action that changes it,
 *    rather than a page of failed requests;
 *  - a worker — an explanation, because the control-plane endpoints will 409
 *    and "Initialise a cluster" would be exactly the wrong advice for a node
 *    that is already in one.
 */
export function SwarmGuard({ children, requiresManager = true }: SwarmGuardProps) {
  const { t } = useTranslation('swarm');
  const { capability, isLoading } = useSwarmCapability();
  const [initOpen, setInitOpen] = useState(false);
  const [joinOpen, setJoinOpen] = useState(false);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <CardSkeleton lines={2} />
        <CardSkeleton lines={6} />
      </div>
    );
  }

  if (!capability.capable) {
    return (
      <>
        <EmptyState
          icon={<IconHexagons className="size-6" />}
          title={t('empty.notSwarm.title')}
          description={t('empty.notSwarm.description')}
        >
          {/* The action is a real Button rather than EmptyState's own
              `primaryAction`, because only Button carries the permission gate:
              a user without swarm.manage sees it disabled with a tooltip
              naming the key, instead of a control that 403s on click. */}
          {/* Same shape as EmptyState's own action row, which this replaces.
              The two ways into a Swarm are starting one and joining one, and
              this empty state is the only place either is reachable when the
              environment is not in a cluster — an operator who has just left
              one is far more often rejoining than founding. */}
          <div className="mt-4 flex flex-wrap items-center justify-center gap-2">
            <Button permission={PERM.SWARM_MANAGE} onClick={() => setInitOpen(true)}>
              {t('actions.initCluster')}
            </Button>
            <Button variant="outline" permission={PERM.SWARM_MANAGE} onClick={() => setJoinOpen(true)}>
              {t('actions.joinCluster')}
            </Button>
          </div>
        </EmptyState>
        <InitClusterDialog open={initOpen} onOpenChange={setInitOpen} />
        <JoinClusterDialog open={joinOpen} onOpenChange={setJoinOpen} />
      </>
    );
  }

  if (requiresManager && !capability.controlAvailable) {
    return (
      <EmptyState
        icon={<IconPlugConnected className="size-6" />}
        title={t('empty.worker.title')}
        description={t('empty.worker.description')}
      />
    );
  }

  return <>{children}</>;
}
