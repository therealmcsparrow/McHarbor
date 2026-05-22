// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { CanvasNode, CanvasEdge, CanvasViewport, CanvasData, CanvasGroup } from '../types';

function generateId(): string {
  return Math.random().toString(36).substring(2, 11);
}

function isPortBlocked(node: CanvasNode | undefined, portKey: string): boolean {
  return node?.blockedPorts?.includes(portKey) ?? false;
}

type CanvasState = {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  groups: CanvasGroup[];
  viewport: CanvasViewport;
  selectedNodeIds: Set<string>;
  selectedEdgeIds: Set<string>;
  snapshotHook: (() => void) | null;
};

type CanvasActions = {
  initCanvas: (data: CanvasData | null) => void;
  getCanvasData: () => CanvasData;
  registerSnapshotHook: (hook: () => void) => void;
  addNode: (node: Omit<CanvasNode, 'id'>) => CanvasNode;
  removeNode: (nodeId: string) => void;
  updateNodeConfig: (nodeId: string, config: Record<string, unknown>) => void;
  updateNodePosition: (nodeId: string, position: { x: number; y: number }) => void;
  updateMultipleNodePositions: (positions: Map<string, { x: number; y: number }>) => void;
  updateNodeLabel: (nodeId: string, label: string) => void;
  updateNodePortLabels: (nodeId: string, portLabels: Record<string, string>) => void;
  updateNodeDebug: (nodeId: string, debug: boolean) => void;
  updateNodeSkip: (nodeId: string, skip: boolean) => void;
  updateNodeDisabled: (nodeId: string, disabled: boolean) => void;
  togglePortBlocked: (nodeId: string, portKey: string) => void;
  addEdge: (edge: Omit<CanvasEdge, 'id'>) => CanvasEdge | null;
  removeEdge: (edgeId: string) => void;
  updateEdgeLabel: (edgeId: string, label: string | null) => void;
  updateEdgeSniffer: (edgeId: string, sniffer: { name: string } | null) => void;
  updateEdgeLabelOffset: (edgeId: string, offset: { x: number; y: number }) => void;
  updateEdgeSnifferOffset: (edgeId: string, offset: { x: number; y: number }) => void;
  duplicateNode: (nodeId: string) => CanvasNode | null;
  deleteSelected: () => void;
  beginDrag: () => void;
  selectNode: (nodeId: string | null, multi?: boolean) => void;
  selectNodesInRect: (nodeIds: string[]) => void;
  selectEdge: (edgeId: string | null, multi?: boolean) => void;
  setViewport: (vp: CanvasViewport) => void;
  createGroup: (name: string, color: string, nodeIds: string[]) => void;
  updateGroup: (groupId: string, updates: { name?: string; color?: string }) => void;
  toggleGroupBlocked: (groupId: string) => void;
  removeGroup: (groupId: string) => void;
  alignNodes: (nodeIds: string[], alignment: string) => void;
};

export const useCanvasStore = create<CanvasState & CanvasActions>((set, get) => ({
  nodes: [],
  edges: [],
  groups: [],
  viewport: { x: 0, y: 0, zoom: 1 },
  selectedNodeIds: new Set(),
  selectedEdgeIds: new Set(),
  snapshotHook: null,

  initCanvas: (data) => set({
    nodes: data?.nodes ?? [],
    edges: data?.edges ?? [],
    groups: data?.groups ?? [],
    viewport: data?.viewport ?? { x: 0, y: 0, zoom: 1 },
    selectedNodeIds: new Set(),
    selectedEdgeIds: new Set(),
  }),

  getCanvasData: () => {
    const { nodes, edges, groups, viewport } = get();
    return { nodes, edges, groups, viewport };
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

  addEdge: (edge) => {
    const { edges, nodes } = get();
    const dup = edges.find(
      (e) => e.sourceNodeId === edge.sourceNodeId && e.sourcePort === edge.sourcePort &&
             e.targetNodeId === edge.targetNodeId && e.targetPort === edge.targetPort,
    );
    if (dup) return null;
    if (wouldCreateCycle(edges, edge.sourceNodeId, edge.targetNodeId)) return null;

    // Verify nodes exist
    const sourceNode = nodes.find((n) => n.id === edge.sourceNodeId);
    const targetNode = nodes.find((n) => n.id === edge.targetNodeId);
    if (!sourceNode || !targetNode) return null;
    if (isPortBlocked(sourceNode, `out:${edge.sourcePort}`) || isPortBlocked(targetNode, `in:${edge.targetPort}`)) {
      return null;
    }

    get().snapshotHook?.();
    const newEdge: CanvasEdge = { ...edge, id: generateId() };
    set((s) => ({ edges: [...s.edges, newEdge] }));
    return newEdge;
  },

  removeEdge: (edgeId) => {
    get().snapshotHook?.();
    set((s) => ({
      edges: s.edges.filter((e) => e.id !== edgeId),
      selectedEdgeIds: new Set([...s.selectedEdgeIds].filter((id) => id !== edgeId)),
    }));
  },

  updateEdgeLabel: (edgeId, label) => {
    get().snapshotHook?.();
    set((s) => ({
      edges: s.edges.map((e) => {
        if (e.id !== edgeId) return e;
        if (label === null || label.trim() === '') {
          const { label: _, ...rest } = e;
          return rest;
        }
        return { ...e, label: label.trim() };
      }),
    }));
  },

  updateEdgeSniffer: (edgeId, sniffer) => {
    get().snapshotHook?.();
    set((s) => ({
      edges: s.edges.map((e) => {
        if (e.id !== edgeId) return e;
        if (sniffer === null) {
          const { sniffer: _, snifferOffset: _so, ...rest } = e;
          return rest;
        }
        return { ...e, sniffer };
      }),
    }));
  },

  updateEdgeLabelOffset: (edgeId, offset) => {
    set((s) => ({
      edges: s.edges.map((e) => (e.id === edgeId ? { ...e, labelOffset: offset } : e)),
    }));
  },

  updateEdgeSnifferOffset: (edgeId, offset) => {
    set((s) => ({
      edges: s.edges.map((e) => (e.id === edgeId ? { ...e, snifferOffset: offset } : e)),
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
        groups = groups.map((g) => ({ ...g, nodeIds: g.nodeIds.filter((id) => id !== nodeId) })).filter((g) => g.nodeIds.length > 0);
      }
      for (const edgeId of selectedEdgeIds) {
        edges = edges.filter((e) => e.id !== edgeId);
      }
      return { nodes, edges, groups, selectedNodeIds: new Set(), selectedEdgeIds: new Set() };
    });
  },

  beginDrag: () => { get().snapshotHook?.(); },

  selectNode: (nodeId, multi = false) => {
    if (nodeId === null) {
      set({ selectedNodeIds: new Set(), selectedEdgeIds: new Set() });
      return;
    }
    if (multi) {
      set((s) => {
        const next = new Set(s.selectedNodeIds);
        if (next.has(nodeId)) next.delete(nodeId); else next.add(nodeId);
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
        if (next.has(edgeId)) next.delete(edgeId); else next.add(edgeId);
        return { selectedEdgeIds: next };
      });
    } else {
      set({ selectedEdgeIds: new Set([edgeId]), selectedNodeIds: new Set() });
    }
  },

  setViewport: (vp) => set({ viewport: vp }),

  createGroup: (name, color, nodeIds) => {
    get().snapshotHook?.();
    set((s) => ({ groups: [...s.groups, { id: generateId(), name, color, nodeIds }] }));
  },

  updateGroup: (groupId, updates) => {
    get().snapshotHook?.();
    set((s) => ({
      groups: s.groups.map((g) =>
        g.id === groupId ? { ...g, ...updates } : g,
      ),
    }));
  },

  toggleGroupBlocked: (groupId) => {
    get().snapshotHook?.();
    set((s) => ({
      groups: s.groups.map((g) =>
        g.id === groupId ? { ...g, blocked: !g.blocked } : g,
      ),
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
            posMap.set(sorted[i]!.id, { x: Math.round(minX + gap * i), y: sorted[i]!.position.y });
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
            posMap.set(sorted[i]!.id, { x: sorted[i]!.position.x, y: Math.round(minY + gap * i) });
          }
        }
        break;
      }
    }

    if (posMap.size > 0) {
      set((s) => ({
        nodes: s.nodes.map((n) => {
          const pos = posMap.get(n.id);
          return pos ? { ...n, position: pos } : n;
        }),
      }));
    }
  },
}));

function wouldCreateCycle(edges: CanvasEdge[], sourceId: string, targetId: string): boolean {
  if (sourceId === targetId) return true;
  const adj: Record<string, string[]> = {};
  for (const e of edges) {
    if (!adj[e.sourceNodeId]) adj[e.sourceNodeId] = [];
    adj[e.sourceNodeId]!.push(e.targetNodeId);
  }
  const visited = new Set<string>();
  const queue = [targetId];
  while (queue.length > 0) {
    const current = queue.shift()!;
    if (current === sourceId) return true;
    if (visited.has(current)) continue;
    visited.add(current);
    for (const n of (adj[current] ?? [])) queue.push(n);
  }
  return false;
}
