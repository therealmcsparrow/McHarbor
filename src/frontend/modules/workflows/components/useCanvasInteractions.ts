// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState, useCallback, useEffect } from 'react';
import i18n from '@core/i18n/i18n';
import { useCanvasStore } from '../stores/canvasStore';
import { useHistoryStore } from '../stores/historyStore';
import { getNodeHeightForPorts, getEffectiveInputPorts, getEffectiveOutputPorts, JUNCTION_SIZE } from '../types';
import { NODE_DEFINITION_MAP } from '../nodes';

const NODE_WIDTH = 224;

type ConnectPreview = { x1: number; y1: number; x2: number; y2: number };
type SelectionRect = { startX: number; startY: number; endX: number; endY: number };

function findNodesInRect(rect: SelectionRect): string[] {
  const minX = Math.min(rect.startX, rect.endX);
  const maxX = Math.max(rect.startX, rect.endX);
  const minY = Math.min(rect.startY, rect.endY);
  const maxY = Math.max(rect.startY, rect.endY);
  if (maxX - minX <= 5 && maxY - minY <= 5) return [];

  return useCanvasStore.getState().nodes.filter((node) => {
    if (node.action === 'junction') {
      return node.position.x + JUNCTION_SIZE > minX && node.position.x < maxX
        && node.position.y + JUNCTION_SIZE > minY && node.position.y < maxY;
    }
    const def = NODE_DEFINITION_MAP[node.action];
    const inputPorts = getEffectiveInputPorts(node, def);
    const outputPorts = getEffectiveOutputPorts(node, def);
    const nh = getNodeHeightForPorts(Math.max(inputPorts.length, outputPorts.length));
    return node.position.x + NODE_WIDTH > minX && node.position.x < maxX
      && node.position.y + nh > minY && node.position.y < maxY;
  }).map((n) => n.id);
}

export function useCanvasInteractions(canvasRef: React.RefObject<HTMLDivElement | null>) {
  const updateNodePosition = useCanvasStore((s) => s.updateNodePosition);
  const updateMultipleNodePositions = useCanvasStore((s) => s.updateMultipleNodePositions);
  const addEdge = useCanvasStore((s) => s.addEdge);
  const deleteSelected = useCanvasStore((s) => s.deleteSelected);
  const beginDrag = useCanvasStore((s) => s.beginDrag);
  const selectNode = useCanvasStore((s) => s.selectNode);
  const selectEdge = useCanvasStore((s) => s.selectEdge);
  const undo = useHistoryStore((s) => s.undo);
  const redo = useHistoryStore((s) => s.redo);

  const panRef = useRef({ active: false, startX: 0, startY: 0 });
  const dragRef = useRef<{ nodeId: string; offsetX: number; offsetY: number } | null>(null);
  const connectRef = useRef<{ active: boolean; nodeId: string; port: string; startX: number; startY: number }>({
    active: false, nodeId: '', port: '', startX: 0, startY: 0,
  });
  const groupDragRef = useRef<{
    groupId: string;
    startX: number;
    startY: number;
    origPositions: Map<string, { x: number; y: number }>;
  } | null>(null);
  const selectionRectRef = useRef<{ startX: number; startY: number } | null>(null);

  const [connectPreview, setConnectPreview] = useState<ConnectPreview | null>(null);
  const [selectionRect, setSelectionRect] = useState<SelectionRect | null>(null);
  const [isPanning, setIsPanning] = useState(false);

  const clientToCanvas = useCallback((clientX: number, clientY: number) => {
    if (!canvasRef.current) return { x: 0, y: 0 };
    const rect = canvasRef.current.getBoundingClientRect();
    const vp = useCanvasStore.getState().viewport;
    return {
      x: (clientX - rect.left - vp.x) / vp.zoom,
      y: (clientY - rect.top - vp.y) / vp.zoom,
    };
  }, [canvasRef]);

  // Keyboard shortcuts
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.target instanceof HTMLSelectElement) return;
      if (e.key === 'Delete' || e.key === 'Backspace') {
        e.preventDefault();
        deleteSelected();
      }
      if ((e.ctrlKey || e.metaKey) && e.key === 'z' && !e.shiftKey) { e.preventDefault(); undo(); }
      if ((e.ctrlKey || e.metaKey) && (e.key === 'y' || (e.key === 'z' && e.shiftKey))) { e.preventDefault(); redo(); }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [deleteSelected, undo, redo]);

  const onWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault();
    const vp = useCanvasStore.getState().viewport;
    const delta = e.deltaY > 0 ? -0.05 : 0.05;
    const newZoom = Math.min(2, Math.max(0.25, vp.zoom + delta));
    useCanvasStore.getState().setViewport({ ...vp, zoom: newZoom });
  }, []);

  const onCanvasMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button === 2) return;
    // Middle mouse button — pan from anywhere on canvas
    if (e.button === 1) {
      e.preventDefault();
      const vp = useCanvasStore.getState().viewport;
      panRef.current = { active: true, startX: e.clientX - vp.x, startY: e.clientY - vp.y };
      setIsPanning(true);
      return;
    }
    const target = e.target as HTMLElement;
    if (target === canvasRef.current || target.classList.contains('canvas-grid')) {
      if (e.shiftKey) {
        const vp = useCanvasStore.getState().viewport;
        panRef.current = { active: true, startX: e.clientX - vp.x, startY: e.clientY - vp.y };
        setIsPanning(true);
      } else {
        const pos = clientToCanvas(e.clientX, e.clientY);
        selectionRectRef.current = { startX: pos.x, startY: pos.y };
        setSelectionRect({ startX: pos.x, startY: pos.y, endX: pos.x, endY: pos.y });
      }
      selectNode(null);
      selectEdge(null);
    }
  }, [selectNode, selectEdge, clientToCanvas, canvasRef]);

  const onCanvasMouseMove = useCallback((e: React.MouseEvent) => {
    if (panRef.current.active) {
      const vp = useCanvasStore.getState().viewport;
      useCanvasStore.getState().setViewport({ ...vp, x: e.clientX - panRef.current.startX, y: e.clientY - panRef.current.startY });
    }
    if (dragRef.current) {
      const pos = clientToCanvas(e.clientX, e.clientY);
      updateNodePosition(dragRef.current.nodeId, { x: Math.round(pos.x - dragRef.current.offsetX), y: Math.round(pos.y - dragRef.current.offsetY) });
    }
    if (groupDragRef.current) {
      const pos = clientToCanvas(e.clientX, e.clientY);
      const dx = pos.x - groupDragRef.current.startX;
      const dy = pos.y - groupDragRef.current.startY;
      const newPositions = new Map<string, { x: number; y: number }>();
      for (const [nodeId, orig] of groupDragRef.current.origPositions) {
        newPositions.set(nodeId, { x: Math.round(orig.x + dx), y: Math.round(orig.y + dy) });
      }
      updateMultipleNodePositions(newPositions);
    }
    if (connectRef.current.active) {
      const pos = clientToCanvas(e.clientX, e.clientY);
      setConnectPreview({ x1: connectRef.current.startX, y1: connectRef.current.startY, x2: pos.x, y2: pos.y });
    }
    if (selectionRectRef.current) {
      const pos = clientToCanvas(e.clientX, e.clientY);
      setSelectionRect({ startX: selectionRectRef.current.startX, startY: selectionRectRef.current.startY, endX: pos.x, endY: pos.y });
    }
  }, [clientToCanvas, updateMultipleNodePositions, updateNodePosition]);

  const onCanvasMouseUp = useCallback(() => {
    if (panRef.current.active) setIsPanning(false);
    panRef.current.active = false;
    dragRef.current = null;
    groupDragRef.current = null;
    if (connectRef.current.active) {
      connectRef.current.active = false;
      setConnectPreview(null);
    }
    if (selectionRectRef.current && selectionRect) {
      const ids = findNodesInRect(selectionRect);
      if (ids.length > 0) useCanvasStore.getState().selectNodesInRect(ids);
      selectionRectRef.current = null;
      setSelectionRect(null);
    }
  }, [selectionRect]);

  const onNodeDragStart = useCallback((nodeId: string, e: React.MouseEvent) => {
    const pos = clientToCanvas(e.clientX, e.clientY);
    const node = useCanvasStore.getState().nodes.find((n) => n.id === nodeId);
    if (!node) return;
    beginDrag();
    dragRef.current = { nodeId, offsetX: pos.x - node.position.x, offsetY: pos.y - node.position.y };
  }, [clientToCanvas, beginDrag]);

  const onPortDragStart = useCallback((nodeId: string, port: string, x: number, y: number) => {
    connectRef.current = { active: true, nodeId, port, startX: x, startY: y };
    setConnectPreview({ x1: x, y1: y, x2: x, y2: y });
  }, []);

  const onPortDrop = useCallback((targetNodeId: string, targetPort: string) => {
    if (connectRef.current.active && connectRef.current.nodeId) {
      addEdge({ sourceNodeId: connectRef.current.nodeId, sourcePort: connectRef.current.port, targetNodeId, targetPort });
    }
    connectRef.current.active = false;
    setConnectPreview(null);
  }, [addEdge]);

  const onEdgeClick = useCallback((edgeId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    selectEdge(edgeId, e.shiftKey);
  }, [selectEdge]);

  const onNodeSelect = useCallback((nodeId: string, e: React.MouseEvent) => {
    selectNode(nodeId, e.shiftKey);
  }, [selectNode]);

  const onGroupDragStart = useCallback((groupId: string, e: React.MouseEvent) => {
    if (e.button !== 0) return;
    e.stopPropagation();
    const group = useCanvasStore.getState().groups.find((g) => g.id === groupId);
    if (!group) return;
    beginDrag();
    const pos = clientToCanvas(e.clientX, e.clientY);
    const nodes = useCanvasStore.getState().nodes;
    const origPositions = new Map<string, { x: number; y: number }>();
    for (const nodeId of group.nodeIds) {
      const node = nodes.find((n) => n.id === nodeId);
      if (node) origPositions.set(nodeId, { ...node.position });
    }
    groupDragRef.current = { groupId, startX: pos.x, startY: pos.y, origPositions };
  }, [clientToCanvas, beginDrag]);

  const onDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    const action = e.dataTransfer.getData('workflow/node-action');
    const def = NODE_DEFINITION_MAP[action];
    if (!def) return;

    const pos = clientToCanvas(e.clientX, e.clientY);
    useCanvasStore.getState().addNode({
      type: def.category === 'trigger' ? 'trigger' : 'action',
      action: def.key,
      label: i18n.t(`nodes:${def.key}.label`, { defaultValue: def.label }),
      config: {},
      position: { x: Math.round(pos.x - NODE_WIDTH / 2), y: Math.round(pos.y - 38) },
    });
  }, [clientToCanvas]);

  const onDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  }, []);

  return {
    isPanning, connectPreview, selectionRect, clientToCanvas, onWheel,
    onCanvasMouseDown, onCanvasMouseMove, onCanvasMouseUp,
    onNodeDragStart, onPortDragStart, onPortDrop,
    onEdgeClick, onNodeSelect, onGroupDragStart, onDrop, onDragOver,
  };
}
