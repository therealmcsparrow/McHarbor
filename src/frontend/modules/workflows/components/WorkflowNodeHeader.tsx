// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX, IconLoader2, IconPlayerTrackNext, IconBan } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import type { CanvasNode, NodeExecutionStatus } from '../types';
import { ct } from '../canvas-theme';

type WorkflowNodeHeaderProps = {
  node: CanvasNode;
  colors: { header: string; border: string; text?: string };
  executionStatus?: NodeExecutionStatus;
};

export function WorkflowNodeHeader({ node, colors, executionStatus }: WorkflowNodeHeaderProps) {
  const { t } = useTranslation('common');
  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-t-lg px-3 py-2 text-xs font-medium',
        colors.text ?? ct.nodeHeaderText,
        colors.header,
        node.skip && 'bg-opacity-60',
      )}
    >
      <span className={cn('flex-1 truncate', node.skip && 'line-through opacity-70')}>{node.label}</span>
      {node.skip && <IconPlayerTrackNext className="size-3 text-warning" title={t('workflows.skipped')} />}
      {node.disabled && <IconBan className="size-3 text-danger" title={t('workflows.disabled')} />}
      {node.debug && !node.skip && !node.disabled && <span className="text-[10px] opacity-60">DBG</span>}
      {executionStatus === 'running' && <IconLoader2 className={`size-3 animate-spin ${ct.text80}`} />}
      {executionStatus === 'completed' && <IconCheck className="size-3 text-success" />}
      {executionStatus === 'failed' && <IconX className="size-3 text-danger" />}
    </div>
  );
}
