// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import type { CanvasNode, CanvasGroup, CanvasViewport } from '../types';
import { getNodeHeightForPorts, getEffectiveInputPorts, getEffectiveOutputPorts } from '../types';
import { resolveNodeDefinition } from '../nodes';

const NODE_WIDTH = 224;
const GROUP_PADDING = 24;
const GROUP_HEADER_HEIGHT = 28;

interface GroupLayerProps {
  groups: CanvasGroup[];
  nodes: CanvasNode[];
  viewport: CanvasViewport;
  onGroupContextMenu: (groupId: string, e: React.MouseEvent) => void;
  onGroupDragStart: (groupId: string, e: React.MouseEvent) => void;
}

export function GroupLayer({
  groups,
  nodes,
  viewport: canvasViewport,
  onGroupContextMenu,
  onGroupDragStart,
}: GroupLayerProps) {
  const { t } = useTranslation('common');

  return (
    <div
      className="[transform-origin:0_0] [transform:var(--vt)]"
      style={cssVars({ vt: `translate(${canvasViewport.x}px, ${canvasViewport.y}px) scale(${canvasViewport.zoom})` })}
    >
      {groups.map((group) => {
        const groupNodes = nodes.filter((n) => group.nodeIds.includes(n.id));
        if (groupNodes.length === 0) return null;

        let minX = Infinity,
          minY = Infinity,
          maxX = -Infinity,
          maxY = -Infinity;
        for (const gn of groupNodes) {
          const def = resolveNodeDefinition(gn.action);
          const inp = getEffectiveInputPorts(gn, def);
          const outp = getEffectiveOutputPorts(gn, def);
          const nh = getNodeHeightForPorts(Math.max(inp.length, outp.length));
          minX = Math.min(minX, gn.position.x);
          minY = Math.min(minY, gn.position.y);
          maxX = Math.max(maxX, gn.position.x + NODE_WIDTH);
          maxY = Math.max(maxY, gn.position.y + nh);
        }

        return (
          <div
            key={group.id}
            className={cn(
              'absolute top-(--y) left-(--x) h-(--h) w-(--w) overflow-hidden rounded-lg border-2 border-(color:--edge) bg-(--fill)',
              group.blocked ? 'border-solid opacity-50' : 'border-dashed',
            )}
            style={cssVars({
              x: `${minX - GROUP_PADDING}px`,
              y: `${minY - GROUP_PADDING - GROUP_HEADER_HEIGHT}px`,
              w: `${maxX - minX + GROUP_PADDING * 2}px`,
              h: `${maxY - minY + GROUP_PADDING * 2 + GROUP_HEADER_HEIGHT}px`,
              edge: group.blocked ? group.color + '30' : group.color + '60',
              fill: group.color + '06',
            })}
            role="presentation"
            onContextMenu={(e) => onGroupContextMenu(group.id, e)}
          >
            <div
              className="flex h-7 cursor-grab items-center gap-1.5 border-b border-(color:--edge) bg-(--fill) px-2.5 active:cursor-grabbing"
              style={cssVars({ fill: group.color + '20', edge: group.color + '30' })}
              role="presentation"
              onMouseDown={(e) => onGroupDragStart(group.id, e)}
            >
              {group.blocked && <IconLock className="size-3 shrink-0 text-(--c)" style={cssVars({ c: group.color })} />}
              <span
                className="truncate text-[10px] leading-none font-semibold text-(--c)"
                style={cssVars({ c: group.color })}
              >
                {group.name}
              </span>
              <span className="ml-auto text-[9px] font-medium text-(--c)" style={cssVars({ c: group.color + '80' })}>
                {t('workflows.nodes', { count: groupNodes.length })}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
