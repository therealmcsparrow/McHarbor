// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useEnvironments } from './hooks/useCompute';
import { useHostEnvironmentNames } from './hooks/useHostEnvironments';
import { OverviewPage } from './pages/OverviewPage';
import { HOST_DETAIL_TABS, HostDetailPage, type HostDetailTab } from './pages/HostDetailPage';
import { PoolsPage } from './pages/PoolsPage';
import { PoolDetailPage } from './pages/PoolDetailPage';
import { NetworksPage } from './pages/NetworksPage';
import { ImagesPage } from './pages/ImagesPage';
import { SetupWizard } from './components/SetupWizard';
import { COMPUTE_PERM } from './permissions';
import type { PluginNavContext } from './index';

/**
 * The plugin's root.
 *
 * The host mounts this once for the whole `/plugins/addon-compute/*` subtree and
 * routes in-plugin navigation through `onNavigate` rather than remounting. That
 * is what keeps the query cache alive across the four sidebar entries — the
 * environment list sits behind every one of them, and remounting per tab would
 * refetch it on each click.
 */

interface AppProps {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
}

export function App({ initial, subscribe }: AppProps) {
  const { t } = useTranslation('compute');
  const [ctx, setCtx] = useState(initial);
  const [wizardFor, setWizardFor] = useState<string | null>(null);

  useEffect(() => subscribe(setCtx), [subscribe]);

  const { data, isLoading, isError, refetch } = useEnvironments();
  const canManage = usePermissionGate(COMPUTE_PERM.MANAGE).allowed;
  const canUpload = usePermissionGate(COMPUTE_PERM.IMAGES_UPLOAD).allowed;

  // Names come from the host rather than from this plugin's API: the backend
  // can only enumerate environment ids. See useHostEnvironments.
  const hostNames = useHostEnvironmentNames();

  // The host picker on every page takes a plain {id, name} list, and only
  // agent-backed hosts belong in it: nothing else can run a guest.
  const environments = useMemo(
    () =>
      (data ?? [])
        .filter((env) => env.unavailableReason !== 'not_agent_environment')
        .map((env) => ({
          id: env.environmentId,
          // Three fallbacks, narrowing to the value that is always present:
          // whatever the API knew, then the host's name, then the raw id.
          name: env.name || hostNames.get(env.environmentId) || env.environmentId,
        })),
    [data, hostNames],
  );

  const navigate = (to: string) => ctx.navigate(`/plugins/addon-compute${to}`);

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (isError) {
    return (
      <div>
        <ErrorState onRetry={() => void refetch()} />
      </div>
    );
  }

  // The splat is the path under the plugin's own mount point.
  const route = (ctx.splat || '').replace(/^\/+/, '');

  const hostSegments = route.startsWith('hosts/') ? route.slice('hosts/'.length).split('/') : null;
  const hostId = hostSegments?.[0] ?? '';
  const hostTab = HOST_DETAIL_TABS.includes((hostSegments?.[1] ?? '') as HostDetailTab)
    ? (hostSegments?.[1] as HostDetailTab)
    : 'overview';

  // The Ctrl+K palette's actions land here as `?action=`: setup on the overview, create on
  // pools, upload on images. A dialog is open for as long as its param is.
  const requested = new URLSearchParams(ctx.search).get('action');
  const clearRequest = () => {
    if (requested) ctx.navigate(ctx.path, { replace: true });
  };
  const setupRequested = route === '' && requested === 'setup' && canManage;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <h1 className="sr-only">{t('title')}</h1>

      {route === '' && <OverviewPage onNavigate={navigate} onSetup={(envId) => setWizardFor(envId || '')} />}
      {hostSegments && hostId !== '' && (
        <HostDetailPage
          envId={hostId}
          tab={hostTab}
          onSelectTab={(next) => navigate(`/hosts/${hostId}/${next}`)}
          onNavigate={navigate}
          onBack={() => navigate('')}
          onSetup={(envId) => setWizardFor(envId || '')}
        />
      )}
      {route === 'pools' && (
        <PoolsPage
          environments={environments}
          onOpenPool={(poolId) => navigate(`/pools/${poolId}`)}
          createRequested={requested === 'create' && canManage}
          onCreateRequestDone={clearRequest}
        />
      )}
      {route.startsWith('pools/') && (
        <PoolDetailPage
          poolId={route.slice('pools/'.length)}
          environments={environments}
          onBack={() => navigate('/pools')}
        />
      )}
      {route === 'networks' && <NetworksPage environments={environments} />}
      {route === 'images' && (
        <ImagesPage uploadRequested={requested === 'upload' && canUpload} onUploadRequestDone={clearRequest} />
      )}

      {(wizardFor !== null || setupRequested) && (
        <SetupWizard
          environments={environments}
          defaultEnvironmentId={wizardFor ?? ''}
          onClose={() => {
            setWizardFor(null);
            clearRequest();
          }}
        />
      )}
    </div>
  );
}
