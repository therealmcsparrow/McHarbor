// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlayFilled } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import type { CanvasNode, NodeExecutionStatus } from '../types';
import { CATEGORY_COLORS, CATEGORY_GLOW_CLASS, FALLBACK_GLOW_CLASS, useNodeDefinition } from '../nodes';
import { ct } from '../canvas-theme';
import { JunctionNode } from './JunctionNode';
import { NodeInputPorts, NodeOutputPorts } from './NodePortHandles';
import { WorkflowNodeHeader } from './WorkflowNodeHeader';
import { useNodePorts } from './useNodePorts';

type WorkflowNodeProps = {
  node: CanvasNode;
  isSelected: boolean;
  isConnecting: boolean;
  isExecuting?: boolean;
  executionStatus?: NodeExecutionStatus;
  onSelect: (e: React.MouseEvent) => void;
  onDragStart: (nodeId: string, e: React.MouseEvent) => void;
  onPortDragStart: (nodeId: string, port: string, x: number, y: number) => void;
  onPortDrop: (nodeId: string, port: string) => void;
  onContextMenu: (nodeId: string, e: React.MouseEvent) => void;
  onPortContextMenu: (nodeId: string, portKey: string, e: React.MouseEvent) => void;
  onRun?: (nodeId: string) => void;
};

export function WorkflowNode({
  node,
  isSelected,
  isConnecting,
  isExecuting,
  executionStatus,
  onSelect,
  onDragStart,
  onPortDragStart,
  onPortDrop,
  onContextMenu,
  onPortContextMenu,
  onRun,
}: WorkflowNodeProps) {
  const { t } = useTranslation('common');
  const definition = useNodeDefinition(node.action);
  const category = definition?.category ?? node.type;
  const colors = CATEGORY_COLORS[category] ?? {
    header: 'bg-info',
    border: 'border-info/30',
  };
  const glowClass = CATEGORY_GLOW_CLASS[category] ?? FALLBACK_GLOW_CLASS;
  const ports = useNodePorts(node, definition, isConnecting);
  const { nodeHeight } = ports;

  const configSummary = useMemo(() => {
    const config = node.config;
    if (!config || Object.keys(config).length === 0) return '';
    return Object.values(config)
      .slice(0, 2)
      .map((v) => String(v).substring(0, 30))
      .join(', ');
  }, [node.config]);

  const executionRingClass =
    executionStatus === 'running'
      ? 'ring-2 ring-info animate-pulse'
      : executionStatus === 'completed'
        ? 'ring-2 ring-success'
        : executionStatus === 'failed'
          ? 'ring-2 ring-danger'
          : '';

  // Selection glow rides a utility class rather than an inline boxShadow,
  // so it resolves through the theme tokens like everything else. The
  // node's `style` keeps only what is genuinely dynamic — its position
  // and height on the canvas.
  const glowClassName = isSelected ? glowClass : '';

  const onNodeMouseDown = (e: React.MouseEvent) => {
    if (e.button === 1) return; // middle-click — let canvas handle pan
    onSelect(e);
    onDragStart(node.id, e);
  };

  const portCallbacks = { onPortDragStart, onPortDrop, onPortContextMenu };

  if (node.action === 'junction') {
    return (
      <JunctionNode
        node={node}
        ports={ports}
        isSelected={isSelected}
        executionStatus={executionStatus}
        executionRingClass={executionRingClass}
        onNodeMouseDown={onNodeMouseDown}
        onContextMenu={onContextMenu}
        {...portCallbacks}
      />
    );
  }

  const runLabel = isExecuting ? t('workflows.workflowRunning') : t('workflows.runWorkflow');

  return (
    <div
      role="presentation"
      className={cn(
        'group absolute flex w-56 cursor-grab flex-col rounded-lg border shadow-lg transition-shadow duration-300 select-none',
        ct.shadowNode,
        colors.border,
        executionRingClass,
        node.disabled && 'opacity-40',
        node.skip && 'border-dashed',
        glowClassName,
        'top-(--y) left-(--x) h-(--h)',
      )}
      style={cssVars({ x: `${node.position.x}px`, y: `${node.position.y}px`, h: `${nodeHeight}px` })}
      onMouseDown={onNodeMouseDown}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onContextMenu(node.id, e);
      }}
    >
      <WorkflowNodeHeader node={node} colors={colors} executionStatus={executionStatus} />

      {/* Body */}
      <div className={`flex-1 rounded-b-lg ${ct.nodeBg} px-3 py-2.5`}>
        {configSummary ? (
          <p className={`truncate text-[0.625rem] ${ct.text40}`}>{configSummary}</p>
        ) : (
          <p className={`text-[0.625rem] ${ct.text25} italic`}>{t('workflows.noConfiguration')}</p>
        )}
      </div>

      <NodeInputPorts node={node} ports={ports} {...portCallbacks} />
      <NodeOutputPorts node={node} ports={ports} {...portCallbacks} />

      {/* Trigger run button */}
      {node.action === 'manual-trigger' && onRun && (
        <Button
          type="button"
          size="icon-sm"
          disabled={isExecuting}
          aria-label={runLabel}
          className={cn(
            `absolute top-(--y) -left-9 ${ct.runBtnText} shadow-md transition-all`,
            isExecuting
              ? 'cursor-not-allowed bg-muted ring-2 ring-success'
              : 'cursor-pointer bg-success hover:scale-110 hover:bg-success',
          )}
          style={cssVars({ y: `${nodeHeight / 2 - 14}px` })}
          title={runLabel}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation();
            onRun(node.id);
          }}
        >
          <IconPlayerPlayFilled className="size-3.5" />
        </Button>
      )}
    </div>
  );
}
