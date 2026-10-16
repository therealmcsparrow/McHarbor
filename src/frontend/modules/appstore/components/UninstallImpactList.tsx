// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useUninstallPreview } from '../hooks/usePlugins';

interface UninstallImpactListProps {
  preview: NonNullable<ReturnType<typeof useUninstallPreview>['data']>;
}

// The "what gets dropped" table inside UninstallConfirmDialog: every
// contribution the plugin registered, plus the permission keys it claims
// and how many roles still reference them.
export function UninstallImpactList({ preview }: UninstallImpactListProps) {
  const { t } = useTranslation('plugins');

  const items: Array<{ key: string; count: number; i18nKey: string }> = [
    { key: 'routes', count: preview.drop_targets.routes, i18nKey: 'confirm.impact.routes' },
    { key: 'menu', count: preview.drop_targets.menu, i18nKey: 'confirm.impact.menu' },
    { key: 'topbar', count: preview.drop_targets.topbar, i18nKey: 'confirm.impact.topbar' },
    {
      key: 'context_menus',
      count: preview.drop_targets.context_menus,
      i18nKey: 'confirm.impact.context_menus',
    },
    { key: 'tabs', count: preview.drop_targets.tabs, i18nKey: 'confirm.impact.tabs' },
    { key: 'widgets', count: preview.drop_targets.widgets, i18nKey: 'confirm.impact.widgets' },
    { key: 'nodes', count: preview.drop_targets.nodes, i18nKey: 'confirm.impact.nodes' },
    {
      key: 'abac_rules',
      count: preview.drop_targets.abac_rules,
      i18nKey: 'confirm.impact.abac_rules',
    },
    {
      key: 'data_tables',
      count: preview.drop_targets.data_tables,
      i18nKey: 'confirm.impact.data_tables',
    },
    {
      key: 'config_keys',
      count: preview.drop_targets.config_keys,
      i18nKey: 'confirm.impact.config_keys',
    },
  ];

  const visibleItems = items.filter((it) => it.count > 0);
  const claimedPerms = preview.permissions.claimed;
  const referencedPerms = preview.permissions.referenced_by_roles;

  return (
    <div className="rounded-md border border-border bg-muted/30 p-3">
      <div className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t('confirm.impact_title')}
      </div>
      {visibleItems.length === 0 && referencedPerms === 0 && claimedPerms === 0 && (
        <div className="text-xs text-muted-foreground">{t('confirm.impact_empty', {})}</div>
      )}
      {visibleItems.length > 0 && (
        <ul className="grid grid-cols-[1fr_auto] gap-x-4 gap-y-1 text-sm">
          {visibleItems.map((it) => (
            <li key={it.key} className="contents">
              <span className="text-muted-foreground">{t(it.i18nKey, { count: it.count })}</span>
              <span className="text-right font-mono text-foreground tabular-nums">{it.count}</span>
            </li>
          ))}
        </ul>
      )}
      {(claimedPerms > 0 || referencedPerms > 0) && (
        <div className="mt-3 border-t border-border/60 pt-2 text-xs">
          {claimedPerms > 0 && (
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">{t('confirm.impact.permissions_claimed', {})}</span>
              <span className="font-mono text-foreground tabular-nums">{claimedPerms}</span>
            </div>
          )}
          {referencedPerms > 0 && (
            <div className="mt-1 flex items-center justify-between text-warning">
              <span>{t('confirm.impact.permissions_referenced', {})}</span>
              <span className="font-mono tabular-nums">{referencedPerms}</span>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
