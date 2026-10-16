// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CanvasEdge } from '../types';
import type { CanvasActions, CanvasGet, CanvasSet } from './canvas-store-types';
import { generateId, isPortBlocked, wouldCreateCycle } from './canvas-store-utils';

type EdgeActions = Pick<
  CanvasActions,
  | 'addEdge'
  | 'removeEdge'
  | 'updateEdgeLabel'
  | 'updateEdgeSniffer'
  | 'updateEdgeLabelOffset'
  | 'updateEdgeSnifferOffset'
>;

export function createEdgeActions(set: CanvasSet, get: CanvasGet): EdgeActions {
  return {
    addEdge: (edge) => {
      const { edges, nodes } = get();
      const dup = edges.find(
        (e) =>
          e.sourceNodeId === edge.sourceNodeId &&
          e.sourcePort === edge.sourcePort &&
          e.targetNodeId === edge.targetNodeId &&
          e.targetPort === edge.targetPort,
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
  };
}
