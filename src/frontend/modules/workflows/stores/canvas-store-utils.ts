// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CanvasNode, CanvasEdge } from '../types';

export function generateId(): string {
  return Math.random().toString(36).substring(2, 11);
}

export function isPortBlocked(node: CanvasNode | undefined, portKey: string): boolean {
  return node?.blockedPorts?.includes(portKey) ?? false;
}

export function wouldCreateCycle(edges: CanvasEdge[], sourceId: string, targetId: string): boolean {
  if (sourceId === targetId) return true;
  const adj: Record<string, string[]> = {};
  for (const e of edges) {
    const neighbors = adj[e.sourceNodeId] ?? [];
    neighbors.push(e.targetNodeId);
    adj[e.sourceNodeId] = neighbors;
  }
  const visited = new Set<string>();
  const queue = [targetId];
  while (queue.length > 0) {
    const current = queue.shift();
    if (!current) {
      continue;
    }
    if (current === sourceId) return true;
    if (visited.has(current)) continue;
    visited.add(current);
    for (const n of adj[current] ?? []) queue.push(n);
  }
  return false;
}

/** Maps each aligned node id to its new position for the given alignment mode. */
export function computeAlignedPositions(
  targets: CanvasNode[],
  alignment: string,
): Map<string, { x: number; y: number }> {
  const xs = targets.map((n) => n.position.x);
  const ys = targets.map((n) => n.position.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  const centerX = (minX + maxX) / 2;
  const centerY = (minY + maxY) / 2;

  const posMap = new Map<string, { x: number; y: number }>();

  switch (alignment) {
    case 'left':
      for (const n of targets) posMap.set(n.id, { x: minX, y: n.position.y });
      break;
    case 'right':
      for (const n of targets) posMap.set(n.id, { x: maxX, y: n.position.y });
      break;
    case 'center-h':
      for (const n of targets) posMap.set(n.id, { x: centerX, y: n.position.y });
      break;
    case 'space-h': {
      const sorted = [...targets].sort((a, b) => a.position.x - b.position.x);
      if (sorted.length > 1) {
        const gap = (maxX - minX) / (sorted.length - 1);
        for (let i = 0; i < sorted.length; i++) {
          const node = sorted[i];
          if (!node) {
            continue;
          }
          posMap.set(node.id, { x: Math.round(minX + gap * i), y: node.position.y });
        }
      }
      break;
    }
    case 'top':
      for (const n of targets) posMap.set(n.id, { x: n.position.x, y: minY });
      break;
    case 'bottom':
      for (const n of targets) posMap.set(n.id, { x: n.position.x, y: maxY });
      break;
    case 'center-v':
      for (const n of targets) posMap.set(n.id, { x: n.position.x, y: centerY });
      break;
    case 'space-v': {
      const sorted = [...targets].sort((a, b) => a.position.y - b.position.y);
      if (sorted.length > 1) {
        const gap = (maxY - minY) / (sorted.length - 1);
        for (let i = 0; i < sorted.length; i++) {
          const node = sorted[i];
          if (!node) {
            continue;
          }
          posMap.set(node.id, { x: node.position.x, y: Math.round(minY + gap * i) });
        }
      }
      break;
    }
  }
  return posMap;
}
