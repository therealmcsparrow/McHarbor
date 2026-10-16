// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconHandClick,
  IconWebhook,
  IconClock,
  IconWorld,
  IconMail,
  IconBell,
  IconGitBranch,
  IconClockPause,
  IconVariable,
  IconFileText,
  IconBox,
  IconBug,
  IconRepeat,
  IconEdit,
  IconHeartRateMonitor,
  IconChartLine,
  IconPuzzle,
} from '@tabler/icons-react';
import { NODE_DEFINITIONS, isNodeDefinitionAvailable } from '@resources/nodes/registry';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import { usePluginLoader } from '@core/plugins/loader';
import { resolvePluginIcon } from '../components/plugin-icons';
import type { CategoryCount } from '../types';

const ICON_MAP: Record<string, ComponentType<{ className?: string }>> = {
  IconHandClick,
  IconWebhook,
  IconClock,
  IconWorld,
  IconMail,
  IconBell,
  IconGitBranch,
  IconClockPause,
  IconVariable,
  IconFileText,
  IconBox,
  IconBug,
  IconRepeat,
  IconEdit,
  IconHeartRateMonitor,
  IconChartLine,
};

const CATEGORY_LABELS: Record<string, string> = {
  trigger: 'workflows.categoryTriggers',
  action: 'workflows.categoryActions',
  logic: 'workflows.categoryLogic',
  utility: 'workflows.categoryUtility',
  integration: 'workflows.categoryIntegrations',
  plugin: 'workflows.categoryPlugin',
};

// One shape for both halves of the list: the built-in catalogue and the
// nodes installed plugins contribute at runtime. Plugin entries carry the
// same `plugin:<slug>:<key>` key the workflow canvas uses, so toggling one
// here writes the state the palette reads.
export type StoreNodeItem = {
  key: string;
  icon: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  category: string;
  source: 'builtin' | 'plugin';
  available: boolean;
};

type NodeStoreFilters = {
  search: string;
  category: string;
  showUnavailable: boolean;
};

/**
 * Assembles the node store list — built-ins plus plugin-contributed nodes —
 * and applies the tab's availability, category and search filters.
 *
 * Kept out of NodesTab so that component is only the layout of the sidebar
 * and the grid.
 */
export function useNodeStoreItems({ search, category, showUnavailable }: NodeStoreFilters) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const capabilities = useNodeAvailability();
  const pluginNodes = usePluginLoader((s) => s.nodes);

  const allNodes = useMemo<StoreNodeItem[]>(() => {
    const builtins: StoreNodeItem[] = NODE_DEFINITIONS.map((node) => ({
      key: node.key,
      icon: ICON_MAP[node.icon] ?? IconPuzzle,
      // i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
      title: tn(`${node.key}.label`, { defaultValue: node.label }),
      // i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
      description: tn(`${node.key}.description`, { defaultValue: node.description }),
      category: node.category,
      source: 'builtin',
      available: isNodeDefinitionAvailable(node, capabilities),
    }));

    // Plugin nodes carry no description column — the runtime stores only
    // key, category, display name, icon and ports — so the card names the
    // plugin that provides them instead.
    const plugins: StoreNodeItem[] = pluginNodes.map((node) => {
      const key = `plugin:${node.plugin_slug}:${node.key}`;
      return {
        key,
        icon: resolvePluginIcon(node.icon),
        title: node.display_name || node.key,
        description: t('store.pluginNodeFrom', { plugin: node.plugin_slug }),
        category: node.category || 'plugin',
        source: 'plugin',
        available: true,
      };
    });

    return [...builtins, ...plugins];
  }, [capabilities, pluginNodes, t, tn]);

  const visibleNodes = useMemo(
    () => (showUnavailable ? allNodes : allNodes.filter((node) => node.available)),
    [allNodes, showUnavailable],
  );

  const categories = useMemo<CategoryCount[]>(() => {
    const counts: Record<string, number> = {};
    for (const n of visibleNodes) {
      counts[n.category] = (counts[n.category] ?? 0) + 1;
    }
    return Object.entries(counts).map(([cat, count]) => ({ category: cat, count }));
  }, [visibleNodes]);

  const categoryLabelMap = useMemo(
    () => Object.fromEntries(Object.entries(CATEGORY_LABELS).map(([k, v]) => [k, t(v)])),
    [t],
  );

  const filtered = useMemo(() => {
    let items = visibleNodes;
    if (category) {
      items = items.filter((n) => n.category === category);
    }
    if (search) {
      const q = search.toLowerCase();
      items = items.filter(
        (n) =>
          n.title.toLowerCase().includes(q) ||
          n.description.toLowerCase().includes(q) ||
          n.category.toLowerCase().includes(q),
      );
    }
    return items;
  }, [search, category, visibleNodes]);

  return { allNodes, visibleNodes, categories, categoryLabelMap, filtered };
}
