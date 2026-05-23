// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { DragEvent } from 'react';
import type { TFunction } from 'i18next';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { CATEGORY_TAG_COLORS, isNodeDefinitionAvailable } from '../nodes';
import { ICON_MAP } from './node-palette-icons';
import type { NodeDefinition } from '../types';

type NodePaletteCategorySectionProps = {
  capabilities: ReturnType<typeof import('@resources/hooks/useNodeAvailability').useNodeAvailability>;
  category: string;
  collapsed: boolean;
  definitions: NodeDefinition[];
  label: string;
  onDragStart: (event: DragEvent, definition: NodeDefinition) => void;
  onToggle: (category: string) => void;
  t: TFunction<'common'>;
  tn: TFunction<'nodes'>;
};

export function NodePaletteCategorySection({
  capabilities,
  category,
  collapsed,
  definitions,
  label,
  onDragStart,
  onToggle,
  t,
  tn,
}: NodePaletteCategorySectionProps) {
  return (
    <div className="mb-1">
      <Button
        variant="ghost"
        onClick={() => onToggle(category)}
        className="h-auto w-full justify-start gap-1.5 rounded-md px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground"
      >
        {collapsed ? <IconChevronRight className="size-3" /> : <IconChevronDown className="size-3" />}
        {label}
        <span className="ml-auto text-[10px] font-normal text-muted-foreground/60">{definitions.length}</span>
      </Button>

      {!collapsed && (
        <div className="space-y-0.5 pb-1">
          {definitions.map((definition) => {
            const Icon = ICON_MAP[definition.icon];
            const tagColor = CATEGORY_TAG_COLORS[definition.category] ?? '';
            const available = isNodeDefinitionAvailable(definition, capabilities);

            return (
              <div
                key={definition.key}
                draggable={available}
                onDragStart={(event) => onDragStart(event, definition)}
                title={available ? undefined : t('workflows.nodeUnavailable')}
                className={cn(
                  'group flex items-start gap-2.5 rounded-lg px-2.5 py-2 transition-colors',
                  available ? 'cursor-grab hover:bg-muted/50 active:cursor-grabbing' : 'cursor-not-allowed opacity-40',
                )}
              >
                <div className={cn('mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-md', tagColor)}>
                  {Icon ? <Icon className="size-3.5" /> : null}
                </div>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-xs font-medium text-foreground">
                    {tn(`${definition.key}.label`, { defaultValue: definition.label })}
                  </p>
                  <p className="truncate text-[10px] text-muted-foreground">
                    {tn(`${definition.key}.description`, { defaultValue: definition.description })}
                  </p>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
