// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useVMs } from './hooks/useVMs';
import { useHostEnvironmentNames } from './hooks/useHostEnvironments';
import { VMListPage } from './pages/VMListPage';
import { VMDetailPage } from './pages/VMDetailPage';
import { CreateVMDialog } from './components/CreateVMDialog';
import type { BackupIntent } from './components/BackupsPanel';
import { VM_PERM } from './permissions';
import type { PluginNavContext } from './index';

/**
 * The plugin's root.
 *
 * The host mounts this once for the whole `/plugins/addon-kvm/*` subtree, so
 * moving between the list, a guest and back arrives through `onNavigate` rather
 * than as a remount. That is what keeps the console alive when an operator
 * switches tabs and comes back — remounting would drop the RFB session and make
 * them reconnect.
 */

interface AppProps {
  initial: PluginNavContext;
  subscribe: (listener: (ctx: PluginNavContext) => void) => () => void;
}

export function App({ initial, subscribe }: AppProps) {
  const { t } = useTranslation('vm');
  const [ctx, setCtx] = useState(initial);
  const [creating, setCreating] = useState(false);

  useEffect(() => subscribe(setCtx), [subscribe]);

  const { data } = useVMs();
  const canCreate = usePermissionGate(VM_PERM.MANAGE).allowed;
  const canImport = usePermissionGate(VM_PERM.IMPORT).allowed;

  // The hosts come from the guests themselves — this plugin cannot read
  // addon-compute's environment table — but their *names* do not: a guest row
  // carries only an id. Those come across the bundle boundary from the host
  // bridge, or the list would read as a column of xids.
  const hostNames = useHostEnvironmentNames();

  const environments = useMemo(() => {
    const seen = new Set<string>();
    for (const vm of data ?? []) seen.add(vm.environmentId);
    return [...seen].map((id) => ({ id, name: hostNames.get(id) || id }));
  }, [data, hostNames]);

  const route = (ctx.splat || '').replace(/^\/+/, '');
  const navigate = (to: string, opts?: { replace?: boolean }) => ctx.navigate(`/plugins/addon-kvm${to}`, opts);

  // `/templates` is a list; anything else non-empty is a guest id.
  const [head = '', tab, intentKind, intentId] = route.split('/');
  const templates = head === 'templates';
  const detailId = head && !templates ? head : '';
  const initialTab = tab === 'backups' ? 'backups' : undefined;
  const backupIntent: BackupIntent | undefined =
    initialTab && (intentKind === 'restore' || intentKind === 'plan') && intentId
      ? { kind: intentKind, id: intentId }
      : undefined;

  // The Ctrl+K palette's actions land here as `?action=create|import`. The dialog is open
  // for as long as the param is, and closing it drops the param.
  const requested = detailId || templates ? null : new URLSearchParams(ctx.search).get('action');
  const clearRequest = () => {
    if (requested) ctx.navigate(ctx.path, { replace: true });
  };
  const showCreate = creating || (requested === 'create' && canCreate);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <h1 className="sr-only">{t('title')}</h1>

      {detailId ? (
        <VMDetailPage
          id={detailId}
          onBack={() => navigate('')}
          initialTab={initialTab}
          backupIntent={backupIntent}
          onIntentHandled={() => navigate(`/${detailId}/backups`, { replace: true })}
        />
      ) : (
        <VMListPage
          templates={templates}
          environments={environments}
          onOpen={(id) => navigate(`/${id}`)}
          onCreate={() => setCreating(true)}
          importRequested={requested === 'import' && canImport}
          onImportRequestDone={clearRequest}
        />
      )}

      {showCreate && (
        <CreateVMDialog
          environments={environments}
          onClose={() => {
            setCreating(false);
            clearRequest();
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
