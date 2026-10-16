// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useContainers } from './hooks/useContainers';
import { ContainerListPage } from './pages/ContainerListPage';
import { ContainerDetailPage } from './pages/ContainerDetailPage';
import { CreateContainerDialog } from './components/CreateContainerDialog';
import { CT_PERM } from './permissions';
import type { PluginNavContext } from './index';

/**
 * The plugin's root.
 *
 * The host mounts this once for the whole `/plugins/addon-lxc/*` subtree, so
 * moving between the list, a container and back arrives through `onNavigate` rather
 * than as a remount. That is what keeps the console alive when an operator
 * switches tabs and comes back — remounting would drop the console session and make
 * them reconnect.
 */

interface AppProps {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
}

export function App({ initial, subscribe }: AppProps) {
  const { t } = useTranslation('ct');
  const [ctx, setCtx] = useState(initial);
  const [creating, setCreating] = useState(false);

  useEffect(() => subscribe(setCtx), [subscribe]);

  const { data } = useContainers();
  const canCreate = usePermissionGate(CT_PERM.MANAGE).allowed;

  // The host list comes from the guests themselves. This plugin cannot read
  // addon-compute's environment table, and the containers already name every host
  // that has one.
  const environments = useMemo(() => {
    const seen = new Map<string, string>();
    for (const container of data ?? []) {
      if (!seen.has(container.environmentId)) {
        seen.set(container.environmentId, container.environmentId);
      }
    }
    return [...seen.entries()].map(([id, label]) => ({ id, name: label }));
  }, [data]);

  const route = (ctx.splat || '').replace(/^\/+/, '');
  const navigate = (to: string) => ctx.navigate(`/plugins/addon-lxc${to}`);

  // `/templates` is a list; anything else non-empty is a container id.
  const templates = route === 'templates';
  const detailId = route && !templates ? route : '';

  // The Ctrl+K palette's create action lands here as `?action=create`. The dialog is open
  // for as long as the param is, and closing it drops the param.
  const requested = detailId || templates ? null : new URLSearchParams(ctx.search).get('action');
  const showCreate = creating || (requested === 'create' && canCreate);

  return (
    <div>
      <h1 className="sr-only">{t('title')}</h1>

      {detailId ? (
        <ContainerDetailPage id={detailId} onBack={() => navigate('')} />
      ) : (
        <ContainerListPage
          templates={templates}
          environments={environments}
          onOpen={(id) => navigate(`/${id}`)}
          onCreate={() => setCreating(true)}
        />
      )}

      {showCreate && (
        <CreateContainerDialog
          environments={environments}
          onClose={() => {
            setCreating(false);
            if (requested) ctx.navigate(ctx.path, { replace: true });
          }}
          onCreated={(id) => {
            setCreating(false);
            navigate(`/${id}`);
          }}
        />
      )}
    </div>
  );
}
