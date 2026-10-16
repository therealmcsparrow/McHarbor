// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CanvasActions, CanvasGet, CanvasSet } from './canvas-store-types';
import { computeAlignedPositions, generateId } from './canvas-store-utils';

type GroupActions = Pick<
  CanvasActions,
  'createGroup' | 'updateGroup' | 'toggleGroupBlocked' | 'removeGroup' | 'alignNodes'
>;

export function createGroupActions(set: CanvasSet, get: CanvasGet): GroupActions {
  return {
    createGroup: (groupName, color, nodeIds) => {
      get().snapshotHook?.();
      set((s) => ({ groups: [...s.groups, { id: generateId(), name: groupName, color, nodeIds }] }));
    },

    updateGroup: (groupId, updates) => {
      get().snapshotHook?.();
      set((s) => ({
        groups: s.groups.map((g) => (g.id === groupId ? { ...g, ...updates } : g)),
      }));
    },

    toggleGroupBlocked: (groupId) => {
      get().snapshotHook?.();
      set((s) => ({
        groups: s.groups.map((g) => (g.id === groupId ? { ...g, blocked: !g.blocked } : g)),
      }));
    },

    removeGroup: (groupId) => {
      get().snapshotHook?.();
      set((s) => ({ groups: s.groups.filter((g) => g.id !== groupId) }));
    },

    alignNodes: (nodeIds, alignment) => {
      const { nodes } = get();
      const targets = nodes.filter((n) => nodeIds.includes(n.id));
      if (targets.length < 2) return;

      get().snapshotHook?.();

      const posMap = computeAlignedPositions(targets, alignment);
      if (posMap.size > 0) {
        set((s) => ({
          nodes: s.nodes.map((n) => {
            const pos = posMap.get(n.id);
            return pos ? { ...n, position: pos } : n;
          }),
        }));
      }
    },
  };
}
