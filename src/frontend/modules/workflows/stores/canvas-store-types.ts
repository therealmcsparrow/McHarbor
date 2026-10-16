// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { StoreApi } from 'zustand';
import type { CanvasNode, CanvasEdge, CanvasViewport, CanvasData, CanvasGroup } from '../types';

export type CanvasState = {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  groups: CanvasGroup[];
  viewport: CanvasViewport;
  selectedNodeIds: Set<string>;
  selectedEdgeIds: Set<string>;
  snapshotHook: (() => void) | null;
};

export type CanvasActions = {
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

export type CanvasStore = CanvasState & CanvasActions;
export type CanvasSet = StoreApi<CanvasStore>['setState'];
export type CanvasGet = StoreApi<CanvasStore>['getState'];
