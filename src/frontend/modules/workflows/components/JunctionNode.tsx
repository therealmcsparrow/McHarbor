// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { IconCheck, IconX, IconLoader2 } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import type { CanvasNode, NodeExecutionStatus } from '../types';
import { JUNCTION_SIZE } from '../types';
import type { NodePortCallbacks } from './NodePortHandles';
import type { NodePorts } from './useNodePorts';

type JunctionNodeProps = NodePortCallbacks & {
  node: CanvasNode;
  ports: NodePorts;
  isSelected: boolean;
  executionStatus?: NodeExecutionStatus;
  executionRingClass: string;
  onNodeMouseDown: (e: React.MouseEvent) => void;
  onContextMenu: (nodeId: string, e: React.MouseEvent) => void;
};

/** Compact dot rendering used for junction nodes. */
export function JunctionNode({
  node,
  ports,
  isSelected,
  executionStatus,
  executionRingClass,
  onNodeMouseDown,
  onContextMenu,
  onPortDragStart,
  onPortDrop,
  onPortContextMenu,
}: JunctionNodeProps) {
  const half = JUNCTION_SIZE / 2;
  const { setHoveredPort } = ports;
  return (
    <div
      role="presentation"
      className="group absolute top-(--y) left-(--x) size-5 cursor-grab select-none"
      style={cssVars({ x: `${node.position.x}px`, y: `${node.position.y}px` })}
      onMouseDown={onNodeMouseDown}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onContextMenu(node.id, e);
      }}
    >
      {/* Dot */}
      <div
        className={cn(
          'size-5 rounded-full border-2 border-border/60 bg-muted transition-shadow duration-200',
          executionRingClass,
          node.disabled && 'opacity-40',
          isSelected && 'shadow-lg ring-2 shadow-muted-foreground/50 ring-muted-foreground/50',
        )}
      />
      {/* Input port (hit area pushed outside the dot) */}
      <div
        role="presentation"
        className="absolute top-(--y) -left-1.5 flex h-4 w-3 cursor-default items-center justify-center"
        style={cssVars({ y: `${half - 8}px` })}
        onMouseEnter={() => setHoveredPort('in:input')}
        onMouseLeave={() => setHoveredPort(null)}
        onMouseUp={(e) => {
          e.stopPropagation();
          if (!node.blockedPorts?.includes('in:input')) {
            onPortDrop(node.id, 'input');
          }
        }}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onPortContextMenu(node.id, 'in:input', e);
        }}
      >
        <div
          className={cn('size-2 rounded-full border-2 transition-all duration-150', ports.inputPortClass('input'))}
        />
      </div>
      {/* Output port (hit area pushed outside the dot) */}
      <div
        role="presentation"
        className="absolute top-(--y) -right-1.5 flex h-4 w-3 cursor-default items-center justify-center"
        style={cssVars({ y: `${half - 8}px` })}
        onMouseEnter={() => setHoveredPort('out:output')}
        onMouseLeave={() => setHoveredPort(null)}
        onMouseDown={(e) => {
          e.stopPropagation();
          if (!node.blockedPorts?.includes('out:output')) {
            onPortDragStart(node.id, 'output', node.position.x + JUNCTION_SIZE, node.position.y + half);
          }
        }}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onPortContextMenu(node.id, 'out:output', e);
        }}
      >
        <div
          className={cn('size-2 rounded-full border-2 transition-all duration-150', ports.outputPortClass('output'))}
        />
      </div>
      {/* Execution status indicators */}
      {executionStatus === 'running' && (
        <IconLoader2 className="absolute -top-2 -right-2 size-3 animate-spin text-info" />
      )}
      {executionStatus === 'completed' && <IconCheck className="absolute -top-2 -right-2 size-3 text-success" />}
      {executionStatus === 'failed' && <IconX className="absolute -top-2 -right-2 size-3 text-danger" />}
    </div>
  );
}
