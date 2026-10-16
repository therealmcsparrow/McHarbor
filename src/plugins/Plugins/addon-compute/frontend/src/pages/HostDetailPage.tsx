// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconAdjustments,
  IconFileText,
  IconHome,
  IconInfoCircle,
  IconRefresh,
  IconServer2,
  IconStethoscope,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { Card } from '@resources/components/ui/Card';
import { Tabs, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import type { StatusTone } from '@resources/theme/status-palette';
import { useHostEnvironmentNames } from '../hooks/useHostEnvironments';
import { useOverview } from '../hooks/useCompute';
import { hostStatus, type HostTone } from '../overview-status';
import { HostOverviewTab } from '../components/host-detail/HostOverviewTab';
import { HostSettingsTab } from '../components/host-detail/HostSettingsTab';
import { HostTestsTab } from '../components/host-detail/HostTestsTab';
import { HostLogsTab } from '../components/host-detail/HostLogsTab';
import type { OverviewEntry } from '../types';

export type HostDetailTab = 'overview' | 'settings' | 'tests' | 'logs';

export const HOST_DETAIL_TABS: HostDetailTab[] = ['overview', 'settings', 'tests', 'logs'];

const STATUS_TONES: Record<HostTone, StatusTone> = {
  success: 'success',
  warning: 'warning',
  destructive: 'danger',
  secondary: 'neutral',
};

interface HostDetailPageProps {
  envId: string;
  tab: HostDetailTab;
  onSelectTab: (tab: HostDetailTab) => void;
  onNavigate: (to: string) => void;
  onBack: () => void;
  onSetup: (environmentId: string) => void;
}

export function HostDetailPage({ envId, tab, onSelectTab, onNavigate, onBack, onSetup }: HostDetailPageProps) {
  const { t } = useTranslation('compute');
  const { data, isLoading, isFetching, isError, refetch } = useOverview();
  const hostNames = useHostEnvironmentNames();

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (isError) {
    return <ErrorState onRetry={() => void refetch()} />;
  }

  const raw = (data?.environments ?? []).find((candidate) => candidate.environmentId === envId);

  if (!raw) {
    return (
      <EmptyState
        icon={<IconServer2 className="size-6" />}
        title={t('hosts.detail.missing.title')}
        description={t('hosts.detail.missing.description')}
        primaryAction={{ label: t('hosts.detail.back'), onClick: onBack }}
      />
    );
  }

  const entry: OverviewEntry = {
    ...raw,
    name: raw.name || hostNames.get(envId) || envId,
  };
  const hostState = hostStatus(entry);

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <HeaderBreadcrumb
        crumbs={[
          { to: '/dashboard', label: <IconHome className="size-3" /> },
          { label: t('overview.title'), onClick: onBack },
          { label: entry.name },
        ]}
      />

      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconServer2 className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {entry.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StatusIndicator
              tone={STATUS_TONES[hostState.tone]}
              label={t(`overview.status.${hostState.key}`)}
              size="sm"
            />
            {entry.capability && (
              <>
                <Badge variant="outline" className="px-1.5 py-0 text-[10px] uppercase">
                  {entry.capability.os}
                </Badge>
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                  {entry.capability.arch}
                </Badge>
              </>
            )}
            <span className="truncate rounded-md bg-muted/60 px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
              {entry.environmentId}
            </span>
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <div className="flex items-center gap-1.5">
          <HeaderActionButton
            tooltip={t('actions.refresh')}
            icon={<IconRefresh className="size-4" />}
            disabled={isFetching}
            onClick={() => void refetch()}
          />
        </div>
      </PageHeadActions>

      <Card className="flex-none overflow-visible">
        <Tabs className="px-5 py-3" value={tab} onValueChange={(next) => onSelectTab(next as HostDetailTab)}>
          <TabsList>
            <TabsTrigger value="overview">
              <IconInfoCircle className="size-4" />
              {t('hosts.detail.tabs.overview')}
            </TabsTrigger>
            <TabsTrigger value="settings">
              <IconAdjustments className="size-4" />
              {t('hosts.detail.tabs.settings')}
            </TabsTrigger>
            <TabsTrigger value="tests">
              <IconStethoscope className="size-4" />
              {t('hosts.detail.tabs.tests')}
            </TabsTrigger>
            <TabsTrigger value="logs">
              <IconFileText className="size-4" />
              {t('hosts.detail.tabs.logs')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </Card>

      <div
        className={tab === 'logs' ? 'flex min-h-0 flex-1 flex-col' : 'min-h-0 flex-1 overflow-auto'}
        data-scroll-exempt
      >
        {tab === 'overview' && <HostOverviewTab entry={entry} onNavigate={onNavigate} />}
        {tab === 'settings' && <HostSettingsTab entry={entry} />}
        {tab === 'tests' && <HostTestsTab entry={entry} onSetup={onSetup} />}
        {tab === 'logs' && <HostLogsTab envId={entry.environmentId} />}
      </div>
    </div>
  );
}
