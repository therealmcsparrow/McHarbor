// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import type { CanvasNode, CanvasGroup, CanvasViewport } from '../types';
import { getNodeHeightForPorts, getEffectiveInputPorts, getEffectiveOutputPorts } from '../types';
import { NODE_DEFINITION_MAP } from '../nodes';

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

export function GroupLayer({ groups, nodes, viewport, onGroupContextMenu, onGroupDragStart }: GroupLayerProps) {
  const { t } = useTranslation('common');

  return (
    <div
      style={{
        transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.zoom})`,
        transformOrigin: '0 0',
      }}
    >
      {groups.map((group) => {
        const groupNodes = nodes.filter((n) => group.nodeIds.includes(n.id));
        if (groupNodes.length === 0) return null;

        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
        for (const gn of groupNodes) {
          const def = NODE_DEFINITION_MAP[gn.action];
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
              'absolute rounded-lg border-2 overflow-hidden',
              group.blocked ? 'border-solid' : 'border-dashed',
            )}
            style={{
              left: minX - GROUP_PADDING,
              top: minY - GROUP_PADDING - GROUP_HEADER_HEIGHT,
              width: maxX - minX + GROUP_PADDING * 2,
              height: maxY - minY + GROUP_PADDING * 2 + GROUP_HEADER_HEIGHT,
              borderColor: group.blocked ? group.color + '30' : group.color + '60',
              backgroundColor: group.color + '06',
              opacity: group.blocked ? 0.5 : 1,
            }}
            onContextMenu={(e) => onGroupContextMenu(group.id, e)}
          >
            <div
              className="flex cursor-grab items-center gap-1.5 px-2.5 active:cursor-grabbing"
              style={{
                height: GROUP_HEADER_HEIGHT,
                backgroundColor: group.color + '20',
                borderBottom: `1px solid ${group.color}30`,
              }}
              onMouseDown={(e) => onGroupDragStart(group.id, e)}
            >
              {group.blocked && (
                <IconLock className="size-3 shrink-0" style={{ color: group.color }} />
              )}
              <span
                className="truncate text-[10px] font-semibold leading-none"
                style={{ color: group.color }}
              >
                {group.name}
              </span>
              <span className="ml-auto text-[9px] font-medium" style={{ color: group.color + '80' }}>
                {t('workflows.nodes', { count: groupNodes.length })}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

