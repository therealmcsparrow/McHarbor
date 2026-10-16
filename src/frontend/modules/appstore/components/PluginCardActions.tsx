// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload, IconRefresh, IconReplace } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useReconcilePlugin } from '../hooks/usePlugins';
import type { PluginEntry } from '../types/plugins';
import { PluginSignatureIcon } from './PluginSignature';

type PluginCardActionsProps = {
  plugin: PluginEntry & { installed?: boolean };
  hasUpdate?: boolean;
  needsRepair?: boolean;
  installedUnsigned?: boolean;
  installedEnabled?: boolean;
  onInstall: () => void;
  onUpdate: () => void;
  onUninstall?: (slug: string) => void;
  onViewDetail?: (plugin: PluginEntry) => void;
};

export function PluginCardActions({
  plugin,
  hasUpdate,
  needsRepair,
  installedUnsigned,
  installedEnabled = true,
  onInstall,
  onUpdate,
  onUninstall,
  onViewDetail,
}: PluginCardActionsProps) {
  const { t } = useTranslation(['common', 'plugins']);
  const reconcile = useReconcilePlugin();

  return (
    <div className="flex items-center justify-between gap-2">
      <PluginSignatureIcon signed={plugin.installed ? !installedUnsigned : !!plugin.signature} />
      <div className="flex shrink-0 items-center justify-end gap-2">
        {onViewDetail && (
          <Button variant="ghost" size="sm" onClick={() => onViewDetail(plugin)} aria-label={t('store.viewDetails')}>
            {t('store.details')}
          </Button>
        )}
        {!plugin.installed && (
          <Button
            permission={PERM.PLUGINS_MANAGE}
            variant={hasUpdate ? 'default' : 'outline'}
            size="sm"
            onClick={onInstall}
            aria-label={t(hasUpdate ? 'store.updateAction' : 'appStore.install')}
            data-testid={`plugin-install-${plugin.slug}`}
          >
            <IconDownload className="size-3.5" />
            {t(hasUpdate ? 'store.updateAction' : 'appStore.install')}
          </Button>
        )}
        {/* Update + Uninstall both show when installed and a newer
            catalog entry exists. The Update button hits the host's
            /api/appstore/plugins/{slug}/update/stream endpoint,
            which Uninstalls then re-installs at the catalog version.
            Falls back to onInstall when the parent didn't wire up
            onUpdate (older builds without /update/stream). */}
        {plugin.installed && needsRepair && (
          <Button
            permission={PERM.PLUGINS_MANAGE}
            variant="default"
            size="sm"
            onClick={onUpdate}
            aria-label={t('repair.action', { ns: 'plugins' })}
            data-testid={`plugin-repair-${plugin.slug}`}
          >
            <IconRefresh className="size-3.5" />
            {t('repair.action', { ns: 'plugins' })}
          </Button>
        )}
        {plugin.installed && hasUpdate && !needsRepair && (
          <Button
            permission={PERM.PLUGINS_MANAGE}
            variant="default"
            size="sm"
            onClick={onUpdate}
            aria-label={t('store.updateAction')}
            data-testid={`plugin-update-${plugin.slug}`}
          >
            <IconDownload className="size-3.5" />
            {t('store.updateAction')}
          </Button>
        )}
        {plugin.installed && (
          <Button
            permission={PERM.PLUGINS_MANAGE}
            variant="outline"
            size="sm"
            onClick={() =>
              reconcile.mutate({
                slug: plugin.slug,
                version: plugin.version,
                enabled: installedEnabled,
              })
            }
            disabled={reconcile.isPending}
            title={t('repair.reconcileHint', { ns: 'plugins', version: plugin.version })}
            data-testid={`plugin-reconcile-${plugin.slug}`}
          >
            <IconReplace className="size-3.5" aria-hidden="true" />
            {t('repair.reconcile', { ns: 'plugins' })}
          </Button>
        )}
        {plugin.installed && onUninstall && (
          <Button
            permission={PERM.PLUGINS_DELETE}
            variant="ghost"
            size="sm"
            onClick={() => onUninstall(plugin.slug)}
            aria-label={t('appStore.uninstall')}
            data-testid={`plugin-uninstall-${plugin.slug}`}
            className="text-destructive hover:text-destructive"
          >
            {t('appStore.uninstall')}
          </Button>
        )}
      </div>
    </div>
  );
}
