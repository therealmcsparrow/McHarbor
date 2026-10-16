// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCanvasStore } from '../stores/canvasStore';
import { getNodeHeightForPorts, getEffectiveInputPorts, getEffectiveOutputPorts, JUNCTION_SIZE } from '../types';
import { resolveNodeDefinition } from '../nodes';
import { NODE_WIDTH } from './useNodePorts';

export type SelectionRect = { startX: number; startY: number; endX: number; endY: number };

export function findNodesInRect(rect: SelectionRect): string[] {
  const minX = Math.min(rect.startX, rect.endX);
  const maxX = Math.max(rect.startX, rect.endX);
  const minY = Math.min(rect.startY, rect.endY);
  const maxY = Math.max(rect.startY, rect.endY);
  if (maxX - minX <= 5 && maxY - minY <= 5) return [];

  return useCanvasStore
    .getState()
    .nodes.filter((node) => {
      if (node.action === 'junction') {
        return (
          node.position.x + JUNCTION_SIZE > minX &&
          node.position.x < maxX &&
          node.position.y + JUNCTION_SIZE > minY &&
          node.position.y < maxY
        );
      }
      const def = resolveNodeDefinition(node.action);
      const inputPorts = getEffectiveInputPorts(node, def);
      const outputPorts = getEffectiveOutputPorts(node, def);
      const nh = getNodeHeightForPorts(Math.max(inputPorts.length, outputPorts.length));
      return (
        node.position.x + NODE_WIDTH > minX &&
        node.position.x < maxX &&
        node.position.y + nh > minY &&
        node.position.y < maxY
      );
    })
    .map((n) => n.id);
}
