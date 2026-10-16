// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { hasPermission, useCurrentUserPermissions } from '@core/auth/useCurrentUserPermissions';
import { usePluginLoader, type PluginTabItem } from './loader';

export type PluginPageTab = {
  /** `plugin:<slug>:<key>` — namespaced so it can never collide with a host tab id. */
  id: PluginTabId;
  slug: string;
  tabKey: string;
  label: string;
  disabled: boolean;
  disabledReason?: string;
};

/** A plugin tab's id. The prefix is what keeps it out of every host tab union. */
export type PluginTabId = `plugin:${string}`;

export function pluginTabId(slug: string, key: string): PluginTabId {
  return `plugin:${slug}:${key}`;
}

export function isPluginTabId(value: string | null | undefined): value is PluginTabId {
  return typeof value === 'string' && value.startsWith('plugin:');
}

export function parsePluginTabId(value: string): { slug: string; tabKey: string } | null {
  if (!isPluginTabId(value)) return null;
  const [, slug, ...rest] = value.split(':');
  if (!slug || rest.length === 0) return null;
  return { slug, tabKey: rest.join(':') };
}

function byOrder(a: PluginTabItem, b: PluginTabItem): number {
  const order = (a.sort_order ?? 0) - (b.sort_order ?? 0);
  return order !== 0 ? order : a.key.localeCompare(b.key);
}

export function usePluginPageTabs(pageId: string): PluginPageTab[] {
  const tabs = usePluginLoader((s) => s.tabs);
  const { t } = useTranslation('common');
  const { data: permissions } = useCurrentUserPermissions();

  return useMemo(() => {
    return tabs
      .filter((tab) => tab.page_id === pageId)
      .sort(byOrder)
      .map((tab) => {
        const required = tab.required_permission;
        const denied = Boolean(required) && !hasPermission(permissions, required!);
        return {
          id: pluginTabId(tab.plugin_slug, tab.key),
          slug: tab.plugin_slug,
          tabKey: tab.key,
          // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: a plugin tab's label key lives in the plugin's own bundle, which may not ship one
          label: t(tab.label_key, { ns: tab.label_ns, defaultValue: tab.key }),
          disabled: denied,
          disabledReason: denied ? t('permission.requires', { permission: required }) : undefined,
        };
      });
  }, [tabs, pageId, t, permissions]);
}
