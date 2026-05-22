// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CanvasEdge, CanvasNode } from '../types';
import { getEffectiveInputPorts, getEffectiveOutputPorts, getNodeHeightForPorts, JUNCTION_SIZE } from '../types';
import { NODE_DEFINITION_MAP } from '../nodes';
import { EdgeLabel } from './EdgeLabel';
import { SnifferIcon, SnifferName } from './SnifferOverlay';

const NODE_WIDTH = 224;

type WorkflowEdgeProps = {
  edge: CanvasEdge;
  nodes: CanvasNode[];
  isSelected: boolean;
  viewportZoom: number;
  isTraversed?: boolean;
  isAnimating?: boolean;
  onClick: (edgeId: string, e: React.MouseEvent) => void;
  onContextMenu: (edgeId: string, e: React.MouseEvent) => void;
};

function getPortPosition(
  node: CanvasNode,
  port: string,
  side: 'output' | 'input',
): { x: number; y: number } | null {
  // Junction: ports at center-left and center-right of the small dot
  if (node.action === 'junction') {
    const half = JUNCTION_SIZE / 2;
    return {
      x: side === 'output' ? node.position.x + JUNCTION_SIZE : node.position.x,
      y: node.position.y + half,
    };
  }

  const def = NODE_DEFINITION_MAP[node.action];
  const inputPorts = getEffectiveInputPorts(node, def);
  const outputPorts = getEffectiveOutputPorts(node, def);
  const ports = side === 'output' ? outputPorts : inputPorts;
  const idx = ports.indexOf(port);
  if (idx === -1) return null;
  const height = getNodeHeightForPorts(Math.max(inputPorts.length, outputPorts.length));
  const spacing = height / (ports.length + 1);
  return {
    x: side === 'output' ? node.position.x + NODE_WIDTH : node.position.x,
    y: node.position.y + spacing * (idx + 1),
  };
}

export function WorkflowEdge({ edge, nodes, isSelected, viewportZoom, isTraversed, isAnimating, onClick, onContextMenu }: WorkflowEdgeProps) {
  const sourceNode = nodes.find((n) => n.id === edge.sourceNodeId);
  const targetNode = nodes.find((n) => n.id === edge.targetNodeId);
  if (!sourceNode || !targetNode) return null;

  const start = getPortPosition(sourceNode, edge.sourcePort, 'output');
  const end = getPortPosition(targetNode, edge.targetPort, 'input');
  if (!start || !end) return null;

  const dx = Math.abs(end.x - start.x);
  const cp = Math.max(50, dx * 0.5);
  const d = `M ${start.x} ${start.y} C ${start.x + cp} ${start.y}, ${end.x - cp} ${end.y}, ${end.x} ${end.y}`;

  // Arrow head
  const angle = Math.atan2(end.y - (end.y), (end.x) - (end.x - cp));
  const arrowSize = 6;
  const ax = end.x - arrowSize * Math.cos(angle - Math.PI / 6);
  const ay = end.y - arrowSize * Math.sin(angle - Math.PI / 6);
  const bx = end.x - arrowSize * Math.cos(angle + Math.PI / 6);
  const by = end.y - arrowSize * Math.sin(angle + Math.PI / 6);

  // Bezier midpoint at t=0.5
  const t = 0.5;
  const cp1x = start.x + cp;
  const cp1y = start.y;
  const cp2x = end.x - cp;
  const cp2y = end.y;
  const mx = (1-t)**3*start.x + 3*(1-t)**2*t*cp1x + 3*(1-t)*t**2*cp2x + t**3*end.x;
  const my = (1-t)**3*start.y + 3*(1-t)**2*t*cp1y + 3*(1-t)*t**2*cp2y + t**3*end.y;

  // Stroke color/style by state
  const strokeColor = isTraversed ? '#22d3ee' : isSelected ? '#60a5fa' : '#64748b';
  const strokeWidth = isTraversed ? 3 : isSelected ? 2.5 : 1.5;
  const strokeOpacity = isTraversed ? 1 : isSelected ? 1 : 0.5;

  return (
    <g>
      {/* Invisible wide hitbox */}
      <path
        d={d}
        fill="none"
        stroke="transparent"
        strokeWidth={16}
        className="cursor-pointer pointer-events-auto"
        onClick={(e) => onClick(edge.id, e)}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          onContextMenu(edge.id, e);
        }}
      />
      {/* Visible edge */}
      <path
        d={d}
        fill="none"
        stroke={strokeColor}
        strokeWidth={strokeWidth}
        strokeOpacity={strokeOpacity}
        strokeDasharray={isTraversed && isAnimating ? '8 4' : undefined}
        className="pointer-events-none transition-colors"
        style={isTraversed && isAnimating ? { animation: 'edge-dash 0.6s linear infinite' } : undefined}
      />
      {/* Arrow */}
      <polygon
        points={`${end.x},${end.y} ${ax},${ay} ${bx},${by}`}
        fill={strokeColor}
        opacity={strokeOpacity}
        className="pointer-events-none"
      />
      {/* Traveling dot when animating */}
      {isTraversed && isAnimating && (
        <>
          <circle r={4} fill="#22d3ee">
            <animateMotion path={d} dur="1.2s" repeatCount="indefinite" />
          </circle>
          <circle r={7} fill="#22d3ee" opacity={0.2}>
            <animateMotion path={d} dur="1.2s" repeatCount="indefinite" />
          </circle>
        </>
      )}
      {/* Inline label */}
      <EdgeLabel edge={edge} mx={mx} my={my - 14} isSelected={isSelected} viewportZoom={viewportZoom} />
      {/* Sniffer: icon on the curve midpoint, name is draggable below */}
      {edge.sniffer && (
        <>
          <SnifferIcon mx={mx} my={my} />
          <SnifferName edge={edge} mx={mx} my={my} viewportZoom={viewportZoom} />
        </>
      )}
    </g>
  );
}
