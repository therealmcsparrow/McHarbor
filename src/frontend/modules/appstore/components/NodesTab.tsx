// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo } from 'react';
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
import {
  CATEGORY_TAG_COLORS,
  NODE_DEFINITIONS,
  isNodeDefinitionAvailable,
} from '@resources/nodes/registry';
import { useNodeCatalogAvailability, useUpdateNodeCatalogEnabled } from '@resources/hooks/useNodeCatalog';
import { Switch } from '@resources/components/ui/Switch';
import { StoreItemCard } from './StoreItemCard';
import { StoreSearchGrid } from './StoreSearchGrid';
import { CategorySidebar } from './CategorySidebar';
import type { CategoryCount } from '../types';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';

const ICON_MAP: Record<string, React.ComponentType<{ className?: string }>> = {
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
};

export function NodesTab() {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [showUnavailable, setShowUnavailable] = useState(false);
  const { data: enabledMap = {} } = useNodeCatalogAvailability();
  const toggleNodeEnabled = useUpdateNodeCatalogEnabled();
  const capabilities = useNodeAvailability();
  const visibleNodes = useMemo(
    () =>
      showUnavailable
        ? NODE_DEFINITIONS
        : NODE_DEFINITIONS.filter((node) => isNodeDefinitionAvailable(node, capabilities)),
    [capabilities, showUnavailable],
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
          tn(`${n.key}.label`, { defaultValue: n.label }).toLowerCase().includes(q) ||
          tn(`${n.key}.description`, { defaultValue: n.description }).toLowerCase().includes(q) ||
          n.category.toLowerCase().includes(q),
      );
    }
    return items;
  }, [search, category, tn, visibleNodes]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-hidden lg:flex-row">
      <CategorySidebar
        categories={categories}
        selected={category}
        onSelect={setCategory}
        totalCount={visibleNodes.length}
        labelMap={categoryLabelMap}
      />

      <div className="min-h-0 flex-1">
        <StoreSearchGrid
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder={t('store.searchNodes')}
          count={filtered.length}
          countText={t('store.nodesCount', { count: filtered.length })}
          emptyMessage={t('store.noNodes')}
          noResultsMessage={t('workflows.noNodesMatch')}
          hasItems={NODE_DEFINITIONS.length > 0}
          actions={
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">
                {t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
              </span>
              <Switch
                checked={showUnavailable}
                onCheckedChange={setShowUnavailable}
                aria-label={t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
              />
            </div>
          }
        >
          {filtered.map((node) => {
            const available = isNodeDefinitionAvailable(node, capabilities);
            return (
              <StoreItemCard
                key={node.key}
                id={node.key}
                icon={ICON_MAP[node.icon] ?? IconPuzzle}
                title={tn(`${node.key}.label`, { defaultValue: node.label })}
                description={tn(`${node.key}.description`, { defaultValue: node.description })}
                category={t(CATEGORY_LABELS[node.category] ?? node.category)}
                categoryClassName={CATEGORY_TAG_COLORS[node.category]}
                source="builtin"
                checked={enabledMap[node.key] !== false}
                onToggle={(id) =>
                  toggleNodeEnabled.mutate({ key: id, enabled: enabledMap[id] === false })
                }
                disabled={!available}
                disabledReason={
                  available
                    ? undefined
                    : t('workflows.nodeUnavailable', {
                        defaultValue:
                          'This node requires a service that is not yet configured. Set it up in Settings first.',
                      })
                }
              />
            );
          })}
        </StoreSearchGrid>
      </div>
    </div>
  );
}

