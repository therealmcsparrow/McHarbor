// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BUILTIN_WIDGET_DEFINITIONS,
  WIDGET_CATEGORIES,
} from '@resources/widgets/registry';
import { useUpdateWidgetCatalogEnabled, useWidgetCatalogDefinitions } from '@resources/hooks/useWidgetCatalog';
import { StoreItemCard } from './StoreItemCard';
import { StoreSearchGrid } from './StoreSearchGrid';
import { CategorySidebar } from './CategorySidebar';
import type { CategoryCount } from '../types';

export function WidgetsTab() {
  const { t } = useTranslation('common');
  const { t: td } = useTranslation('dashboard');
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const { data: widgetCatalog } = useWidgetCatalogDefinitions();
  const toggleWidgetEnabled = useUpdateWidgetCatalogEnabled();
  const allDefinitions = BUILTIN_WIDGET_DEFINITIONS;
  const enabledMap = useMemo(
    () =>
      Object.fromEntries(
        (widgetCatalog ?? []).map((definition) => [definition.key, definition.enabled !== false]),
      ),
    [widgetCatalog],
  );

  const categoryLabelMap = useMemo(
    () => Object.fromEntries(WIDGET_CATEGORIES.map((c) => [c.id, td(c.labelKey)])),
    [td],
  );

  const categories = useMemo<CategoryCount[]>(() => {
    const counts: Record<string, number> = {};
    for (const w of allDefinitions) {
      counts[w.category] = (counts[w.category] ?? 0) + 1;
    }
    return Object.entries(counts).map(([cat, count]) => ({ category: cat, count }));
  }, [allDefinitions]);

  const filtered = useMemo(() => {
    let items = allDefinitions;
    if (category) {
      items = items.filter((w) => w.category === category);
    }
    if (search) {
      const q = search.toLowerCase();
      items = items.filter((w) => {
        const label = td(w.labelKey);
        const desc = td(w.descriptionKey);
        return (
          label.toLowerCase().includes(q) ||
          desc.toLowerCase().includes(q) ||
          w.category.toLowerCase().includes(q)
        );
      });
    }
    return items;
  }, [search, category, allDefinitions, td]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-hidden lg:flex-row">
      <CategorySidebar
        categories={categories}
        selected={category}
        onSelect={setCategory}
        totalCount={allDefinitions.length}
        labelMap={categoryLabelMap}
      />

      <div className="min-h-0 flex-1">
        <StoreSearchGrid
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder={t('store.searchWidgets')}
          count={filtered.length}
          countText={t('store.widgetsCount', { count: filtered.length })}
          emptyMessage={t('store.noWidgets')}
          noResultsMessage={t('store.noWidgets')}
          hasItems={allDefinitions.length > 0}
        >
          {filtered.map((widget) => (
            <StoreItemCard
              key={widget.id}
              id={widget.id}
              icon={widget.icon}
              title={td(widget.labelKey)}
              description={td(widget.descriptionKey)}
              category={categoryLabelMap[widget.category] ?? widget.category}
              source="builtin"
              checked={enabledMap[widget.id] !== false}
              onToggle={(id) =>
                toggleWidgetEnabled.mutate({ key: id, enabled: enabledMap[id] === false })
              }
              extra={
                <span className="text-xs text-muted-foreground">
                  {widget.defaultSize.w}x{widget.defaultSize.h}
                </span>
              }
            />
          ))}
        </StoreSearchGrid>
      </div>
    </div>
  );
}

