// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import type { CanvasNode } from '../types';
import { getEffectiveInputPorts, getEffectiveOutputPorts, getNodeHeightForPorts, JUNCTION_SIZE } from '../types';
import type { useNodeDefinition } from '../nodes';
import { ct } from '../canvas-theme';

export const NODE_WIDTH = 224;

const BLOCKED_PORT_CLASS = 'border-dashed border-danger/40 bg-danger/10';
const HOVERED_PORT_CLASS = 'border-info bg-info/30 ring-2 ring-info/60 scale-150';

/** Port layout, hover state and class helpers shared by the regular and junction node renderers. */
export function useNodePorts(
  node: CanvasNode,
  definition: ReturnType<typeof useNodeDefinition>,
  isConnecting: boolean,
) {
  const inputPorts = useMemo(() => getEffectiveInputPorts(node, definition), [node, definition]);
  const outputPorts = useMemo(() => getEffectiveOutputPorts(node, definition), [node, definition]);
  const nodeHeight = getNodeHeightForPorts(Math.max(inputPorts.length, outputPorts.length));
  const [hoveredPort, setHoveredPort] = useState<string | null>(null);

  const isRedPort = (port: string) => {
    if (port === 'error') return true;
    if (node.action !== 'condition') return false;
    return port === outputPorts[outputPorts.length - 1];
  };

  const isGreenPort = (port: string) => node.action === 'loop' && port === 'done';

  const getPortLabel = (portKey: string, fallback: string) => node.portLabels?.[portKey] || fallback;

  const inputPortClass = (port: string) => {
    if (node.blockedPorts?.includes(`in:${port}`)) return BLOCKED_PORT_CLASS;
    if (hoveredPort === `in:${port}`) return HOVERED_PORT_CLASS;
    if (isConnecting) return 'border-info bg-info/20 ring-2 ring-info/50 animate-pulse scale-125';
    return `${ct.border30} ${ct.portBg} group-hover:border-foreground/60 group-hover:bg-foreground/20`;
  };

  const outputPortClass = (port: string) => {
    if (node.blockedPorts?.includes(`out:${port}`)) return BLOCKED_PORT_CLASS;
    if (hoveredPort === `out:${port}`) return HOVERED_PORT_CLASS;
    if (isRedPort(port)) return 'border-danger/50 bg-danger/20';
    if (isGreenPort(port)) return 'border-success/50 bg-success/20';
    return `${ct.border30} ${ct.portBg} group-hover:border-foreground/60 group-hover:bg-foreground/20`;
  };

  const getOutputPortPos = (index: number) => ({
    x: node.position.x + (node.action === 'junction' ? JUNCTION_SIZE : NODE_WIDTH),
    y:
      node.position.y +
      ((node.action === 'junction' ? JUNCTION_SIZE : nodeHeight) / (outputPorts.length + 1)) * (index + 1),
  });

  const getPortOffset = (count: number, index: number) => (nodeHeight / (count + 1)) * (index + 1) - 12;

  return {
    inputPorts,
    outputPorts,
    nodeHeight,
    hoveredPort,
    setHoveredPort,
    getPortLabel,
    inputPortClass,
    outputPortClass,
    getOutputPortPos,
    getPortOffset,
  };
}

export type NodePorts = ReturnType<typeof useNodePorts>;
