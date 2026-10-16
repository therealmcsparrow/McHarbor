// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconAlertTriangle,
  IconBox,
  IconKey,
  IconLock,
  IconLogout,
  IconNetwork,
  IconServer2,
  IconStack2,
} from '@tabler/icons-react';
import { PERM } from '../permissions';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { statusSoft } from '@resources/theme/status-palette';
import { useSwarmCapability } from '../hooks/useSwarmCapability';
import { useSwarmOverview } from '../hooks/useSwarmCluster';
import { SwarmGuard } from '../components/SwarmGuard';
import { JoinClusterDialog } from '../components/ClusterLifecycleDialogs';
import { LeaveClusterDialog } from '../components/ClusterLeaveDialog';
import { JoinTokensDialog, UnlockKeyDialog } from '../components/ClusterSecretDialogs';
import { ClusterIdentityCard, StatCard, SmallStat } from './SwarmOverviewParts';

export default function SwarmOverviewPage() {
  return (
    // The overview is the one page a worker can partly answer, so it does not
    // demand the control plane; the panels below fall back to the capability
    // card when the listings are unavailable.
    <SwarmGuard requiresManager={false}>
      <SwarmOverview />
    </SwarmGuard>
  );
}

function SwarmOverview() {
  const { t } = useTranslation('swarm');
  const { capability } = useSwarmCapability();
  const overview = useSwarmOverview(capability.controlAvailable);
  const [joinOpen, setJoinOpen] = useState(false);
  const [leaveOpen, setLeaveOpen] = useState(false);
  const [tokensOpen, setTokensOpen] = useState(false);
  const [unlockOpen, setUnlockOpen] = useState(false);

  const data = overview.data;

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('overview.title')}
        description={t('overview.description')}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" permission={PERM.SWARM_MANAGE} onClick={() => setTokensOpen(true)}>
              <IconKey className="size-4" />
              {t('actions.joinTokens')}
            </Button>
            <Button variant="outline" permission={PERM.SWARM_MANAGE} onClick={() => setUnlockOpen(true)}>
              <IconLock className="size-4" />
              {t('actions.unlockKey')}
            </Button>
            <Button variant="outline" permission={PERM.SWARM_MANAGE} onClick={() => setJoinOpen(true)}>
              {t('actions.joinCluster')}
            </Button>
            <Button variant="destructive" permission={PERM.SWARM_MANAGE} onClick={() => setLeaveOpen(true)}>
              <IconLogout className="size-4" />
              {t('actions.leaveCluster')}
            </Button>
          </div>
        }
      />

      <ClusterIdentityCard />

      {overview.isLoading && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <CardSkeleton lines={2} />
          <CardSkeleton lines={2} />
          <CardSkeleton lines={2} />
          <CardSkeleton lines={2} />
        </div>
      )}

      {overview.error && (
        <ErrorState title={t('errors.overviewFailed')} error={overview.error} onRetry={() => void overview.refetch()} />
      )}

      {data && (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              to="nodes"
              icon={<IconServer2 className="size-5" />}
              label={t('overview.stats.nodes')}
              value={String(data.nodesTotal)}
              detail={t('overview.stats.nodesDetail', {
                managers: data.managers,
                workers: data.workers,
              })}
              tone={data.nodesDown > 0 ? 'danger' : 'success'}
            />
            <StatCard
              to="services"
              icon={<IconBox className="size-5" />}
              label={t('overview.stats.services')}
              value={String(data.servicesTotal)}
              detail={t('overview.stats.servicesDetail', {
                converged: data.servicesConverged,
                degraded: data.servicesDegraded,
              })}
              tone={data.servicesDegraded > 0 ? 'warning' : 'success'}
            />
            <StatCard
              to="tasks"
              icon={<IconStack2 className="size-5" />}
              label={t('overview.stats.tasks')}
              value={`${data.tasksRunning}/${data.tasksDesired}`}
              detail={t('overview.stats.tasksDetail', { failed: data.tasksFailed })}
              tone={data.tasksFailed > 0 ? 'danger' : 'success'}
            />
            <StatCard
              to="networks"
              icon={<IconNetwork className="size-5" />}
              label={t('overview.stats.networks')}
              value={String(data.networks)}
              detail={t('overview.stats.networksDetail', { stacks: data.stacks })}
              tone="info"
            />
          </div>

          {data.nodesDown > 0 && (
            <div className={`flex items-start gap-3 rounded-xl p-4 ${statusSoft('danger')}`}>
              <IconAlertTriangle className="mt-0.5 size-5 shrink-0" />
              <div>
                <p className="text-sm font-medium">{t('overview.alerts.nodesDown.title')}</p>
                <p className="text-sm opacity-90">
                  {t('overview.alerts.nodesDown.description', { count: data.nodesDown })}
                </p>
              </div>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-3">
            <SmallStat label={t('overview.stats.stacks')} value={String(data.stacks)} to="stacks" />
            <SmallStat label={t('overview.stats.secrets')} value={String(data.secrets)} to="secrets" />
            <SmallStat label={t('overview.stats.configs')} value={String(data.configs)} to="configs" />
          </div>
        </>
      )}

      <JoinClusterDialog open={joinOpen} onOpenChange={setJoinOpen} />
      <LeaveClusterDialog open={leaveOpen} onOpenChange={setLeaveOpen} />
      <JoinTokensDialog open={tokensOpen} onOpenChange={setTokensOpen} />
      <UnlockKeyDialog open={unlockOpen} onOpenChange={setUnlockOpen} />
    </div>
  );
}
