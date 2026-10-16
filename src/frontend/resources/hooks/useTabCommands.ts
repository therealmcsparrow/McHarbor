// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, type ComponentType } from 'react';
import { useCommandRegistry, type CommandEntry } from '@resources/hooks/useCommandRegistry';

export type TabCommand<TabId extends string> = {
  id: TabId;
  label: string;
  icon?: ComponentType<{ className?: string }>;
  keywords?: string[];
};

type UseTabCommandsOptions<TabId extends string> = {
  pageId: string;
  pageLabel: string;
  tabs: readonly TabCommand<TabId>[];
  onTabChange: (tab: TabId) => void;
  keywords?: string[];
};

const noKeywords: string[] = [];

export function useTabCommands<TabId extends string>({
  pageId,
  pageLabel,
  tabs,
  onTabChange,
  keywords = noKeywords,
}: UseTabCommandsOptions<TabId>) {
  const commands = useMemo<CommandEntry[]>(
    () =>
      tabs.map((tab, index) => ({
        id: `tab:${pageId}:${tab.id}`,
        label: `${pageLabel}: ${tab.label}`,
        icon: tab.icon,
        group: 'jump',
        to: '',
        action: () => onTabChange(tab.id),
        keywords: ['tab', pageLabel, tab.id, tab.label, ...keywords, ...(tab.keywords ?? [])],
        priority: 20 - index,
      })),
    [keywords, onTabChange, pageId, pageLabel, tabs],
  );

  useCommandRegistry(commands);
}
