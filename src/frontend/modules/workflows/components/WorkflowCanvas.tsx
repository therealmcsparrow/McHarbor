// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useCallback } from 'react';
import { useCanvasStore } from '../stores/canvasStore';
import { useExecutionStore } from '../stores/executionStore';
import { WorkflowNode } from './WorkflowNode';
import { WorkflowEdge } from './WorkflowEdge';
import { GroupLayer } from './GroupLayer';
import { GroupDialog } from './GroupDialog';
import { EditGroupDialog } from './EditGroupDialog';
import { SelectionOverlay } from './SelectionOverlay';
import { CanvasEmptyState } from './CanvasEmptyState';
import { CanvasContextMenus } from './CanvasContextMenus';
import { useCanvasInteractions } from './useCanvasInteractions';
import { useCanvasContextMenus } from './useCanvasContextMenus';
import { ct } from '../canvas-theme';

interface WorkflowCanvasProps {
  onExecute?: (triggerNodeId: string) => void;
}

export function WorkflowCanvas({ onExecute }: WorkflowCanvasProps) {
  const canvasRef = useRef<HTMLDivElement>(null);
  const nodes = useCanvasStore((s) => s.nodes);
  const edges = useCanvasStore((s) => s.edges);
  const viewport = useCanvasStore((s) => s.viewport);
  const selectedNodeIds = useCanvasStore((s) => s.selectedNodeIds);
  const selectedEdgeIds = useCanvasStore((s) => s.selectedEdgeIds);
  const groups = useCanvasStore((s) => s.groups);
  const isExecuting = useExecutionStore((s) => s.isExecuting);
  const nodeStates = useExecutionStore((s) => s.nodeStates);
  const traversedEdges = useExecutionStore((s) => s.traversedEdges);
  const animatingEdges = useExecutionStore((s) => s.animatingEdges);

  const interactions = useCanvasInteractions(canvasRef);
  const menus = useCanvasContextMenus(onExecute);

  const handleCanvasContextMenu = useCallback((e: React.MouseEvent) => {
    menus.onCanvasContextMenu(e, canvasRef.current);
  }, [menus]);

  return (
    <div
      ref={canvasRef}
      className={`canvas-grid relative h-full w-full overflow-hidden ${ct.canvasBg} ${interactions.isPanning ? 'cursor-grabbing' : ''}`}
      style={{
        backgroundImage: `radial-gradient(circle, ${ct.gridDot} 1px, transparent 1px)`,
        backgroundSize: `${20 * viewport.zoom}px ${20 * viewport.zoom}px`,
        backgroundPosition: `${viewport.x}px ${viewport.y}px`,
      }}
      onMouseDown={(e) => { if (e.button === 1) e.preventDefault(); interactions.onCanvasMouseDown(e); }}
      onMouseMove={interactions.onCanvasMouseMove}
      onMouseUp={interactions.onCanvasMouseUp}
      onMouseLeave={interactions.onCanvasMouseUp}
      onAuxClick={(e) => e.preventDefault()}
      onWheel={interactions.onWheel}
      onDrop={interactions.onDrop}
      onDragOver={interactions.onDragOver}
      onContextMenu={handleCanvasContextMenu}
    >
      {/* Edges SVG layer */}
      <svg className="pointer-events-none absolute inset-0" style={{ overflow: 'visible' }}>
        <g transform={`translate(${viewport.x}, ${viewport.y}) scale(${viewport.zoom})`}>
          {edges.map((edge) => (
            <WorkflowEdge
              key={edge.id}
              edge={edge}
              nodes={nodes}
              isSelected={selectedEdgeIds.has(edge.id)}
              viewportZoom={viewport.zoom}
              isTraversed={traversedEdges.has(edge.id)}
              isAnimating={animatingEdges.has(edge.id)}
              onClick={interactions.onEdgeClick}
              onContextMenu={menus.onEdgeContextMenu}
            />
          ))}
          {interactions.connectPreview && (
            <line
              x1={interactions.connectPreview.x1}
              y1={interactions.connectPreview.y1}
              x2={interactions.connectPreview.x2}
              y2={interactions.connectPreview.y2}
              stroke="#60a5fa"
              strokeWidth={2}
              strokeDasharray="6 3"
              opacity={0.7}
            />
          )}
        </g>
      </svg>

      <GroupLayer groups={groups} nodes={nodes} viewport={viewport} onGroupContextMenu={menus.onGroupContextMenu} onGroupDragStart={interactions.onGroupDragStart} />

      {/* Nodes layer */}
      <div style={{ transform: `translate(${viewport.x}px, ${viewport.y}px) scale(${viewport.zoom})`, transformOrigin: '0 0' }}>
        {nodes.map((node) => (
          <WorkflowNode
            key={node.id}
            node={node}
            isSelected={selectedNodeIds.has(node.id)}
            isConnecting={interactions.connectPreview !== null}
            isExecuting={isExecuting && nodeStates[node.id] === 'running'}
            executionStatus={nodeStates[node.id]}
            onSelect={(e) => interactions.onNodeSelect(node.id, e)}
            onDragStart={interactions.onNodeDragStart}
            onPortDragStart={interactions.onPortDragStart}
            onPortDrop={interactions.onPortDrop}
            onContextMenu={menus.onNodeContextMenu}
            onPortContextMenu={menus.onPortContextMenu}
            onRun={menus.onNodeRun}
          />
        ))}
      </div>

      {interactions.selectionRect && <SelectionOverlay rect={interactions.selectionRect} viewport={viewport} />}
      {nodes.length === 0 && <CanvasEmptyState />}

      <CanvasContextMenus menus={menus} />

      {menus.groupDialog && (
        <GroupDialog
          groupName={menus.groupName}
          onGroupNameChange={menus.setGroupName}
          onCommit={menus.commitGroup}
          onCancel={() => { menus.setGroupDialog(null); menus.setGroupName(''); }}
          inputRef={menus.groupInputRef}
        />
      )}

      {menus.editGroupDialog && (
        <EditGroupDialog
          dialog={menus.editGroupDialog}
          onChange={menus.setEditGroupDialog}
          onCommit={menus.commitEditGroup}
          onCancel={() => menus.setEditGroupDialog(null)}
          inputRef={menus.editGroupInputRef}
        />
      )}
    </div>
  );
}
