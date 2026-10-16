// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CATEGORY_TAG_COLORS } from '@resources/nodes/registry';
import { useNodeCatalogAvailability, useUpdateNodeCatalogEnabled } from '@resources/hooks/useNodeCatalog';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { useNodeStoreItems } from '../hooks/useNodeStoreItems';
import { StoreItemCard } from './StoreItemCard';
import { StoreSearchGrid } from './StoreSearchGrid';
import { CategorySidebar } from './CategorySidebar';

export function NodesTab() {
  const { t } = useTranslation('common');
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const [showUnavailable, setShowUnavailable] = useState(false);
  const { data: enabledMap = {} } = useNodeCatalogAvailability();
  const toggleNodeEnabled = useUpdateNodeCatalogEnabled();
  // Turning a node on or off writes the catalog, which is store_nodes.manage.
  const manageGate = usePermissionGate(PERM.STORE_NODES_MANAGE);

  const { allNodes, visibleNodes, categories, categoryLabelMap, filtered } = useNodeStoreItems({
    search,
    category,
    showUnavailable,
  });

  const showUnavailableLabel = t('workflows.showUnavailableNodes');

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
          hasItems={allNodes.length > 0}
          actions={
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">{showUnavailableLabel}</span>
              <Switch
                checked={showUnavailable}
                onCheckedChange={setShowUnavailable}
                aria-label={showUnavailableLabel}
              />
            </div>
          }
        >
          {filtered.map((node) => (
            <StoreItemCard
              key={node.key}
              id={node.key}
              icon={node.icon}
              title={node.title}
              description={node.description}
              category={categoryLabelMap[node.category] ?? node.category}
              categoryClassName={CATEGORY_TAG_COLORS[node.category]}
              source={node.source}
              checked={enabledMap[node.key] !== false}
              onToggle={(id) => toggleNodeEnabled.mutate({ key: id, enabled: enabledMap[id] === false })}
              disabled={!node.available || manageGate.denied}
              disabledReason={
                manageGate.denied ? manageGate.reason : node.available ? undefined : t('workflows.nodeUnavailable')
              }
            />
          ))}
        </StoreSearchGrid>
      </div>
    </div>
  );
}
