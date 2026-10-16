// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { CanvasNode } from '../types';
import type { CanvasStore } from './canvas-store-types';
import { createEdgeActions } from './canvas-edge-actions';
import { createGroupActions } from './canvas-group-actions';
import { generateId } from './canvas-store-utils';

export const useCanvasStore = create<CanvasStore>((set, get) => ({
  ...createEdgeActions(set, get),
  ...createGroupActions(set, get),

  nodes: [],
  edges: [],
  groups: [],
  viewport: { x: 0, y: 0, zoom: 1 },
  selectedNodeIds: new Set(),
  selectedEdgeIds: new Set(),
  snapshotHook: null,

  initCanvas: (data) =>
    set({
      nodes: data?.nodes ?? [],
      edges: data?.edges ?? [],
      groups: data?.groups ?? [],
      viewport: data?.viewport ?? { x: 0, y: 0, zoom: 1 },
      selectedNodeIds: new Set(),
      selectedEdgeIds: new Set(),
    }),

  getCanvasData: () => {
    const { nodes, edges, groups, viewport: currentViewport } = get();
    return { nodes, edges, groups, viewport: currentViewport };
  },

  registerSnapshotHook: (hook) => set({ snapshotHook: hook }),

  addNode: (node) => {
    get().snapshotHook?.();
    const newNode: CanvasNode = { ...node, id: generateId() };
    set((s) => ({ nodes: [...s.nodes, newNode] }));
    return newNode;
  },

  removeNode: (nodeId) => {
    get().snapshotHook?.();
    set((s) => ({
      nodes: s.nodes.filter((n) => n.id !== nodeId),
      edges: s.edges.filter((e) => e.sourceNodeId !== nodeId && e.targetNodeId !== nodeId),
      groups: s.groups
        .map((g) => ({ ...g, nodeIds: g.nodeIds.filter((id) => id !== nodeId) }))
        .filter((g) => g.nodeIds.length > 0),
      selectedNodeIds: new Set([...s.selectedNodeIds].filter((id) => id !== nodeId)),
    }));
  },

  updateNodeConfig: (nodeId, config) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, config } : n)) }));
  },

  updateNodePosition: (nodeId, position) => {
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, position } : n)) }));
  },

  updateMultipleNodePositions: (positions) => {
    set((s) => ({
      nodes: s.nodes.map((n) => {
        const pos = positions.get(n.id);
        return pos ? { ...n, position: pos } : n;
      }),
    }));
  },

  updateNodeLabel: (nodeId, label) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, label } : n)) }));
  },

  updateNodePortLabels: (nodeId, portLabels) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, portLabels } : n)) }));
  },

  updateNodeDebug: (nodeId, debug) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, debug } : n)) }));
  },

  updateNodeSkip: (nodeId, skip) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, skip } : n)) }));
  },

  updateNodeDisabled: (nodeId, disabled) => {
    get().snapshotHook?.();
    set((s) => ({ nodes: s.nodes.map((n) => (n.id === nodeId ? { ...n, disabled } : n)) }));
  },

  togglePortBlocked: (nodeId, portKey) => {
    get().snapshotHook?.();
    set((s) => ({
      nodes: s.nodes.map((n) => {
        if (n.id !== nodeId) return n;
        const blocked = n.blockedPorts ?? [];
        return {
          ...n,
          blockedPorts: blocked.includes(portKey) ? blocked.filter((p) => p !== portKey) : [...blocked, portKey],
        };
      }),
    }));
  },

  duplicateNode: (nodeId) => {
    const original = get().nodes.find((n) => n.id === nodeId);
    if (!original) return null;
    return get().addNode({
      type: original.type,
      action: original.action,
      label: original.label + ' (Copy)',
      config: { ...original.config },
      position: { x: original.position.x + 40, y: original.position.y + 40 },
      ...(original.portLabels ? { portLabels: { ...original.portLabels } } : {}),
      ...(original.debug ? { debug: true } : {}),
    });
  },

  deleteSelected: () => {
    const { selectedNodeIds, selectedEdgeIds } = get();
    if (selectedNodeIds.size === 0 && selectedEdgeIds.size === 0) return;
    get().snapshotHook?.();
    set((s) => {
      let nodes = s.nodes;
      let edges = s.edges;
      let groups = s.groups;
      for (const nodeId of selectedNodeIds) {
        nodes = nodes.filter((n) => n.id !== nodeId);
        edges = edges.filter((e) => e.sourceNodeId !== nodeId && e.targetNodeId !== nodeId);
        groups = groups
          .map((g) => ({ ...g, nodeIds: g.nodeIds.filter((id) => id !== nodeId) }))
          .filter((g) => g.nodeIds.length > 0);
      }
      for (const edgeId of selectedEdgeIds) {
        edges = edges.filter((e) => e.id !== edgeId);
      }
      return { nodes, edges, groups, selectedNodeIds: new Set(), selectedEdgeIds: new Set() };
    });
  },

  beginDrag: () => {
    get().snapshotHook?.();
  },

  selectNode: (nodeId, multi = false) => {
    if (nodeId === null) {
      set({ selectedNodeIds: new Set(), selectedEdgeIds: new Set() });
      return;
    }
    if (multi) {
      set((s) => {
        const next = new Set(s.selectedNodeIds);
        if (next.has(nodeId)) next.delete(nodeId);
        else next.add(nodeId);
        return { selectedNodeIds: next };
      });
    } else {
      set({ selectedNodeIds: new Set([nodeId]), selectedEdgeIds: new Set() });
    }
  },

  selectNodesInRect: (nodeIds) => {
    set({ selectedNodeIds: new Set(nodeIds), selectedEdgeIds: new Set() });
  },

  selectEdge: (edgeId, multi = false) => {
    if (edgeId === null) {
      set({ selectedNodeIds: new Set(), selectedEdgeIds: new Set() });
      return;
    }
    if (multi) {
      set((s) => {
        const next = new Set(s.selectedEdgeIds);
        if (next.has(edgeId)) next.delete(edgeId);
        else next.add(edgeId);
        return { selectedEdgeIds: next };
      });
    } else {
      set({ selectedEdgeIds: new Set([edgeId]), selectedNodeIds: new Set() });
    }
  },

  setViewport: (vp) => set({ viewport: vp }),
}));
