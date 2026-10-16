// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { cn } from '@resources/utils/cn';
import type { CanvasNode } from '../types';
import { ct } from '../canvas-theme';
import type { NodePorts } from './useNodePorts';

export type NodePortCallbacks = {
  onPortDragStart: (nodeId: string, port: string, x: number, y: number) => void;
  onPortDrop: (nodeId: string, port: string) => void;
  onPortContextMenu: (nodeId: string, portKey: string, e: React.MouseEvent) => void;
};

type PortHandlesProps = NodePortCallbacks & {
  node: CanvasNode;
  ports: NodePorts;
};

export function NodeInputPorts({ node, ports, onPortDrop, onPortContextMenu }: PortHandlesProps) {
  const { inputPorts, hoveredPort, setHoveredPort } = ports;
  return inputPorts.map((port, i) => (
    <div
      key={`in-${port}`}
      role="presentation"
      className="absolute top-(--y) -left-3 flex h-6 w-6 cursor-default items-center justify-center"
      style={cssVars({ y: `${ports.getPortOffset(inputPorts.length, i)}px` })}
      onMouseEnter={() => setHoveredPort(`in:${port}`)}
      onMouseLeave={() => setHoveredPort(null)}
      onMouseDown={(e) => e.stopPropagation()}
      onMouseUp={(e) => {
        e.stopPropagation();
        if (!node.blockedPorts?.includes(`in:${port}`)) onPortDrop(node.id, port);
      }}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onPortContextMenu(node.id, `in:${port}`, e);
      }}
    >
      <div className={cn('size-2.5 rounded-full border-2 transition-all duration-150', ports.inputPortClass(port))} />
      {hoveredPort === `in:${port}` && (
        <div
          className={`absolute right-full mr-1.5 rounded whitespace-nowrap ${ct.tooltipBg} px-1.5 py-0.5 text-[9px] ${ct.text80} pointer-events-none`}
        >
          {ports.getPortLabel(`in:${port}`, port)}
        </div>
      )}
    </div>
  ));
}

export function NodeOutputPorts({ node, ports, onPortDragStart, onPortContextMenu }: PortHandlesProps) {
  const { outputPorts, hoveredPort, setHoveredPort } = ports;
  return outputPorts.map((port, i) => (
    <div
      key={`out-${port}`}
      role="presentation"
      className="absolute top-(--y) -right-3 flex h-6 w-6 cursor-default items-center justify-center"
      style={cssVars({ y: `${ports.getPortOffset(outputPorts.length, i)}px` })}
      onMouseEnter={() => setHoveredPort(`out:${port}`)}
      onMouseLeave={() => setHoveredPort(null)}
      onMouseDown={(e) => {
        e.stopPropagation();
        if (!node.blockedPorts?.includes(`out:${port}`)) {
          const pos = ports.getOutputPortPos(i);
          onPortDragStart(node.id, port, pos.x, pos.y);
        }
      }}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onPortContextMenu(node.id, `out:${port}`, e);
      }}
    >
      <div className={cn('size-2.5 rounded-full border-2 transition-all duration-150', ports.outputPortClass(port))} />
      {hoveredPort === `out:${port}` && (
        <div
          className={`absolute left-full ml-1.5 rounded whitespace-nowrap ${ct.tooltipBg} px-1.5 py-0.5 text-[9px] ${ct.text80} pointer-events-none`}
        >
          {ports.getPortLabel(`out:${port}`, port)}
        </div>
      )}
    </div>
  ));
}
