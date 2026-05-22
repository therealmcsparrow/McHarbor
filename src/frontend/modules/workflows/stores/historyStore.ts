// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { CanvasNode, CanvasEdge, CanvasGroup } from '../types';
import { useCanvasStore } from './canvasStore';

type HistorySnapshot = {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  groups: CanvasGroup[];
};

const MAX_HISTORY = 50;

type HistoryState = {
  undoStack: HistorySnapshot[];
  redoStack: HistorySnapshot[];
};

type HistoryActions = {
  init: () => void;
  undo: () => void;
  redo: () => void;
  clearHistory: () => void;
};

export const useHistoryStore = create<HistoryState & HistoryActions>((set, get) => ({
  undoStack: [],
  redoStack: [],

  init: () => {
    const canvas = useCanvasStore.getState();
    canvas.registerSnapshotHook(() => {
      const { nodes, edges, groups } = useCanvasStore.getState();
      const snap: HistorySnapshot = {
        nodes: JSON.parse(JSON.stringify(nodes)),
        edges: JSON.parse(JSON.stringify(edges)),
        groups: JSON.parse(JSON.stringify(groups)),
      };
      set((s) => ({
        undoStack: [...s.undoStack.slice(-(MAX_HISTORY - 1)), snap],
        redoStack: [],
      }));
    });
    set({ undoStack: [], redoStack: [] });
  },

  undo: () => {
    const { undoStack } = get();
    if (undoStack.length === 0) return;

    const { nodes, edges, groups } = useCanvasStore.getState();
    const current: HistorySnapshot = {
      nodes: JSON.parse(JSON.stringify(nodes)),
      edges: JSON.parse(JSON.stringify(edges)),
      groups: JSON.parse(JSON.stringify(groups)),
    };

    const prev = undoStack[undoStack.length - 1];
    set((s) => ({
      undoStack: s.undoStack.slice(0, -1),
      redoStack: [...s.redoStack, current],
    }));

    if (prev) {
      useCanvasStore.setState({ nodes: prev.nodes, edges: prev.edges, groups: prev.groups });
    }
  },

  redo: () => {
    const { redoStack } = get();
    if (redoStack.length === 0) return;

    const { nodes, edges, groups } = useCanvasStore.getState();
    const current: HistorySnapshot = {
      nodes: JSON.parse(JSON.stringify(nodes)),
      edges: JSON.parse(JSON.stringify(edges)),
      groups: JSON.parse(JSON.stringify(groups)),
    };

    const next = redoStack[redoStack.length - 1];
    set((s) => ({
      redoStack: s.redoStack.slice(0, -1),
      undoStack: [...s.undoStack, current],
    }));

    if (next) {
      useCanvasStore.setState({ nodes: next.nodes, edges: next.edges, groups: next.groups });
    }
  },

  clearHistory: () => set({ undoStack: [], redoStack: [] }),
}));
