// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Switch } from '@resources/components/ui/Switch';
import { getAllNodeDefinitions, isNodeDefinitionAvailable } from '../nodes';
import { useCustomNodeSync } from '../hooks/useCustomNodeSync';
import { useNodeCatalogAvailability } from '../hooks/useNodeCatalog';
import { useCustomNodeRegistry } from '../stores/custom-node-registry';
import { useNodeAvailability } from '@resources/hooks/useNodeAvailability';
import type { NodeDefinition } from '../types';
import { NodePaletteCategorySection } from './NodePaletteCategorySection';

const CATEGORY_ORDER = ['trigger', 'action', 'logic', 'utility', 'integration'];

export function NodePalette() {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  const [search, setSearch] = useState('');
  const [showUnavailable, setShowUnavailable] = useState(false);

  // Fetch and sync custom nodes from the API
  useCustomNodeSync();
  const customNodes = useCustomNodeRegistry((s) => s.customNodes);
  const { data: enabledMap = {} } = useNodeCatalogAvailability();
  const capabilities = useNodeAvailability();
  const allNodes = useMemo(
    () => getAllNodeDefinitions(customNodes).filter((definition) => enabledMap[definition.key] !== false),
    [customNodes, enabledMap],
  );
  const visibleNodes = useMemo(
    () =>
      showUnavailable
        ? allNodes
        : allNodes.filter((definition) => isNodeDefinitionAvailable(definition, capabilities)),
    [allNodes, capabilities, showUnavailable],
  );

  const CATEGORY_LABELS: Record<string, string> = {
    trigger: t('workflows.categoryTriggers'),
    action: t('workflows.categoryActions'),
    logic: t('workflows.categoryLogic'),
    utility: t('workflows.categoryUtility'),
    integration: t('workflows.categoryIntegrations'),
  };
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const grouped = useMemo(() => {
    const lc = search.toLowerCase();
    const filtered = lc
      ? visibleNodes.filter((d) => {
          const label = tn(`${d.key}.label`, { defaultValue: d.label });
          const desc = tn(`${d.key}.description`, { defaultValue: d.description });
          return label.toLowerCase().includes(lc) || desc.toLowerCase().includes(lc);
        })
      : visibleNodes;

    const map: Record<string, NodeDefinition[]> = {};
    for (const def of filtered) {
      const categoryNodes = map[def.category] ?? [];
      categoryNodes.push(def);
      map[def.category] = categoryNodes;
    }
    // Sort nodes ascending by translated label within each category
    for (const cat of Object.keys(map)) {
      const categoryNodes = map[cat];
      if (!categoryNodes) continue;
      categoryNodes.sort((a, b) => {
        const la = tn(`${a.key}.label`, { defaultValue: a.label });
        const lb = tn(`${b.key}.label`, { defaultValue: b.label });
        return la.localeCompare(lb);
      });
    }
    return map;
  }, [search, tn, visibleNodes]);

  const toggleCategory = (cat: string) => {
    setCollapsed((prev) => ({ ...prev, [cat]: !prev[cat] }));
  };

  const onDragStart = (e: React.DragEvent, def: NodeDefinition) => {
    if (!isNodeDefinitionAvailable(def, capabilities)) {
      e.preventDefault();
      return;
    }
    e.dataTransfer.setData('workflow/node-action', def.key);
    e.dataTransfer.effectAllowed = 'copy';
  };

  return (
    <div className="flex w-[260px] flex-col border-r border-border bg-card">
      <div className="border-b border-border p-3">
        <div className="relative">
          <IconSearch className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="text"
            placeholder={t('workflows.searchNodes')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-8 pl-8 text-xs"
          />
        </div>
        <div className="mt-2 flex items-center justify-between gap-3">
          <span className="text-[10px] text-muted-foreground">
            {t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
          </span>
          <Switch
            checked={showUnavailable}
            onCheckedChange={setShowUnavailable}
            aria-label={t('workflows.showUnavailableNodes', { defaultValue: 'Show unavailable nodes' })}
          />
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-2">
        {CATEGORY_ORDER.map((cat) => {
          const defs = grouped[cat];
          if (!defs || defs.length === 0) return null;
          const isCollapsed = collapsed[cat] ?? false;

          return (
            <NodePaletteCategorySection
              key={cat}
              capabilities={capabilities}
              category={cat}
              collapsed={isCollapsed}
              definitions={defs}
              label={CATEGORY_LABELS[cat] ?? cat}
              onDragStart={onDragStart}
              onToggle={toggleCategory}
              t={t}
              tn={tn}
            />
          );
        })}

        {Object.keys(grouped).length === 0 && (
          <p className="py-8 text-center text-xs text-muted-foreground">{t('workflows.noNodesMatch')}</p>
        )}
      </div>
    </div>
  );
}

