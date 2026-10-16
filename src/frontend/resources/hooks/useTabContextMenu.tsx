// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';
import { useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';
import { appToast } from '@resources/utils/toast';
import { IconCopy, IconExternalLink, IconRefresh, IconSwitchHorizontal } from '@tabler/icons-react';
import type { TabContextMenuItem } from '@resources/components/ui/Tabs';
import type { TabCommand } from './useTabCommands';

type UseTabContextMenuOptions<TabId extends string> = {
  tabs: readonly TabCommand<TabId>[];
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
  getTabHref?: (tab: TabId) => string | undefined;
  includeLinkActions?: boolean;
  onReloadTab?: (tab: TabId) => void;
};

function fallbackTabHref(pathname: string, search: string, tab: string) {
  const params = new URLSearchParams(search);
  params.set('tab', tab);
  const query = params.toString();
  return `${pathname}${query ? `?${query}` : ''}`;
}

export function useTabContextMenu<TabId extends string>({
  tabs,
  activeTab,
  onTabChange,
  getTabHref,
  includeLinkActions = true,
  onReloadTab,
}: UseTabContextMenuOptions<TabId>) {
  const { t } = useTranslation('common');
  const loc = useLocation();

  return useCallback(
    (tab: TabId): TabContextMenuItem[] => {
      const tabInfo = tabs.find((candidate) => candidate.id === tab);
      const tabLabel = tabInfo?.label ?? tab;
      const relativeHref = getTabHref?.(tab) ?? fallbackTabHref(loc.pathname, loc.search, tab);
      const items: TabContextMenuItem[] = [];

      if (tab !== activeTab) {
        items.push({
          id: 'switch-tab',
          label: t('tabContext.switchToTab', { tab: tabLabel }),
          icon: <IconSwitchHorizontal className="size-4" aria-hidden="true" />,
          onSelect: () => onTabChange(tab),
        });
      }

      if (onReloadTab) {
        items.push({
          id: 'reload-tab',
          label: t('tabContext.reloadTab', { tab: tabLabel }),
          icon: <IconRefresh className="size-4" aria-hidden="true" />,
          onSelect: () => onReloadTab(tab),
        });
      }

      if (includeLinkActions) {
        const href = new URL(relativeHref, window.location.origin).toString();
        items.push(
          {
            id: 'copy-tab-link',
            label: t('tabContext.copyLink'),
            icon: <IconCopy className="size-4" aria-hidden="true" />,
            onSelect: () => {
              navigator.clipboard
                .writeText(href)
                .then(() => appToast.success(t('tabContext.copySuccess')))
                .catch(() => appToast.error(t('tabContext.copyFailed')));
            },
          },
          {
            id: 'open-tab-link',
            label: t('tabContext.openInNewTab'),
            icon: <IconExternalLink className="size-4" aria-hidden="true" />,
            onSelect: () => window.open(href, '_blank', 'noopener,noreferrer'),
          },
        );
      }

      return items;
    },
    [activeTab, getTabHref, includeLinkActions, loc.pathname, loc.search, onReloadTab, onTabChange, t, tabs],
  );
}
