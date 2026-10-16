// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The per-tab context menu, rebuilt on the plugin's own navigation.
//
// The host has `useTabContextMenu`, and the core page used it. It cannot be
// reused here: it derives "open in new tab" hrefs from react-router's
// `useLocation()`, and this bundle has no Router context — the hook would
// throw. Pointing it at a guessed URL instead would be worse than no menu,
// which is why the first cut of this plugin shipped without one.
//
// The information it needs is all in `PluginNavContext`: where we are, how to
// navigate, and (through `pluginHref`) what the absolute URL of any tab is. So
// the menu is rebuilt rather than borrowed, and the entries match the host's
// so the two feel like one product.

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { TabContextMenuItem } from '@resources/components/ui/Tabs';
import { pluginHref } from '../nav-context';
import type { TabId } from '../nav';

/**
 * Returns a builder: give it a tab, get that tab's menu items.
 *
 * Mirrors the shape of the host's `useTabContextMenu` so a future consolidation
 * is a rename rather than a rewrite.
 */
export function useTabMenu(
  activeTab: TabId,
  onTabChange: (tab: TabId) => void,
  onReload: () => void,
): (tab: TabId) => TabContextMenuItem[] {
  const { t } = useTranslation('kubernetes');

  return useCallback(
    (tab: TabId): TabContextMenuItem[] => {
      const href = pluginHref(tab === 'overview' ? '' : tab);
      const isActive = tab === activeTab;

      const items: TabContextMenuItem[] = [
        {
          id: 'open',
          label: t('tabs.menu.open'),
          onSelect: () => onTabChange(tab),
          disabled: isActive,
        },
        {
          id: 'open-new-tab',
          label: t('tabs.menu.openInNewTab'),
          // A real absolute URL, so the browser opens the host route and the
          // host router mounts this plugin at that path — the same thing a
          // middle-click on the sidebar entry does.
          onSelect: () => window.open(href, '_blank', 'noopener,noreferrer'),
        },
        {
          id: 'copy-link',
          label: t('tabs.menu.copyLink'),
          onSelect: () => {
            void navigator.clipboard?.writeText(new URL(href, window.location.origin).toString());
          },
        },
      ];

      // Reloading a tab you are not looking at is meaningless — the summary is
      // one query behind every tab, so "reload" is always "reload the cluster".
      if (isActive) {
        items.push({ id: 'reload', label: t('tabs.menu.reload'), onSelect: onReload });
      }

      return items;
    },
    [activeTab, onReload, onTabChange, t],
  );
}
