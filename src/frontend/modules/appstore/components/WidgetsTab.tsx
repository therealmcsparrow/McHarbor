// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useMemo, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { BUILTIN_WIDGET_DEFINITIONS, WIDGET_CATEGORIES } from '@resources/widgets/registry';
import { useUpdateWidgetCatalogEnabled, useWidgetCatalogDefinitions } from '@resources/hooks/useWidgetCatalog';
import { usePluginWidgets } from '@core/plugins/loader';
import { Badge } from '@resources/components/ui/Badge';
import { parsePluginWidgetSize } from '@core/plugins/bridge-widgets';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { PERM } from '@core/auth/permissionKeys';
import { resolvePluginIcon } from './plugin-icons';
import { StoreItemCard } from './StoreItemCard';
import { WidgetCatalogSection } from './WidgetCatalogSection';
import { StoreSearchGrid } from './StoreSearchGrid';
import { CategorySidebar } from './CategorySidebar';
import type { CategoryCount } from '../types';

// One shape for both halves of the list: the built-in catalogue and the
// widgets installed plugins contribute at runtime. Plugin entries carry the
// same `plugin:<slug>:<key>` id the dashboard registry uses, so toggling one
// here writes the state the picker reads.
type StoreWidgetItem = {
  id: string;
  icon: ComponentType<{ className?: string }>;
  title: string;
  description: string;
  category: string;
  source: 'builtin' | 'plugin' | 'downloaded';
  size: { w: number; h: number };
};

export function WidgetsTab() {
  const { t } = useTranslation('common');
  const { t: td } = useTranslation('dashboard');
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('');
  const { data: widgetCatalog } = useWidgetCatalogDefinitions();
  const toggleWidgetEnabled = useUpdateWidgetCatalogEnabled();
  // Turning a widget on or off writes the catalog: store_widgets.manage.
  const manageGate = usePermissionGate(PERM.STORE_WIDGETS_MANAGE);
  const pluginWidgets = usePluginWidgets();
  const enabledMap = useMemo(
    () => Object.fromEntries((widgetCatalog ?? []).map((definition) => [definition.key, definition.enabled !== false])),
    [widgetCatalog],
  );

  const allDefinitions = useMemo<StoreWidgetItem[]>(() => {
    const builtins: StoreWidgetItem[] = BUILTIN_WIDGET_DEFINITIONS.map((widget) => ({
      id: widget.id,
      icon: widget.icon,
      title: td(widget.labelKey),
      description: td(widget.descriptionKey),
      category: widget.category,
      source: 'builtin',
      size: widget.defaultSize,
    }));

    // `title` and `settings.description` are i18n keys in `ns:path` form —
    // i18next echoes an unknown key, so a plugin shipping plain text still
    // reads correctly.
    const plugins: StoreWidgetItem[] = pluginWidgets.map((widget) => ({
      id: `plugin:${widget.plugin_slug}:${widget.key}`,
      icon: resolvePluginIcon(widget.settings?.icon),
      title: t(widget.title),
      description: t(widget.settings?.description ?? widget.title),
      category: 'plugin',
      source: 'plugin',
      size: parsePluginWidgetSize(widget.default_size, { w: 4, h: 3 }),
    }));

    const downloaded: StoreWidgetItem[] = (widgetCatalog ?? [])
      .filter((widget) => widget.source === 'downloaded')
      .map((widget) => ({
        id: widget.key,
        icon: resolvePluginIcon(widget.icon),
        title: widget.label,
        description: widget.description,
        category: 'plugin',
        source: 'downloaded',
        size: widget.defaultSize,
      }));

    return [...builtins, ...plugins, ...downloaded];
  }, [pluginWidgets, widgetCatalog, t, td]);

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
      items = items.filter(
        (w) =>
          w.title.toLowerCase().includes(q) ||
          w.description.toLowerCase().includes(q) ||
          w.category.toLowerCase().includes(q),
      );
    }
    return items;
  }, [search, category, allDefinitions]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-6 overflow-hidden lg:flex-row">
      <CategorySidebar
        categories={categories}
        selected={category}
        onSelect={setCategory}
        totalCount={allDefinitions.length}
        labelMap={categoryLabelMap}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto">
        <WidgetCatalogSection />
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
              title={widget.title}
              description={widget.description}
              category={categoryLabelMap[widget.category] ?? widget.category}
              source={widget.source}
              checked={enabledMap[widget.id] !== false}
              onToggle={(id) => toggleWidgetEnabled.mutate({ key: id, enabled: enabledMap[id] === false })}
              disabled={manageGate.denied}
              disabledReason={manageGate.denied ? manageGate.reason : undefined}
              extra={
                <Badge variant="info">
                  {widget.size.w}x{widget.size.h}
                </Badge>
              }
            />
          ))}
        </StoreSearchGrid>
      </div>
    </div>
  );
}
