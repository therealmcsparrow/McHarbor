// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconPlayerStop, IconRotate, IconArrowDown, IconArrowUp, IconTrash } from '@tabler/icons-react';
import type { BatchAction } from '@resources/layout/table-page/types';
import { isProtectedStack } from '@core/utils/protection';
import type { StackInfo } from '@resources/hooks/useStacks';

type StackBatchActionsParams = {
  tc: (key: string) => string;
  onAction: (name: string, action: string) => void;
  onDelete: (name: string) => void;
  onUpdateSelected?: (rows: StackInfo[]) => void;
  onReinstallSelected?: (rows: StackInfo[]) => void;
};

export function getStackBatchActions({
  tc,
  onAction,
  onDelete,
  onUpdateSelected,
  onReinstallSelected,
}: StackBatchActionsParams): BatchAction[] {
  return [
    {
      label: tc('batch.updateSelected'),
      icon: IconArrowUp,
      iconClassName: 'text-success',
      variant: 'default',
      onClick: (rows) => onUpdateSelected?.(rows as StackInfo[]),
    },
    {
      label: tc('batch.reinstallSelected'),
      icon: IconRotate,
      iconClassName: 'text-info',
      variant: 'default',
      onClick: (rows) => onReinstallSelected?.(rows as StackInfo[]),
    },
    {
      label: tc('batch.stop'),
      icon: IconPlayerStop,
      iconClassName: 'text-warning',
      variant: 'default',
      onClick: (rows) => {
        for (const row of rows as StackInfo[]) {
          if (isProtectedStack(row)) continue;
          onAction(row.name, 'stop');
        }
      },
    },
    {
      label: tc('batch.restart'),
      icon: IconRotate,
      iconClassName: 'text-info',
      variant: 'default',
      onClick: (rows) => {
        for (const row of rows as StackInfo[]) {
          if (isProtectedStack(row)) continue;
          onAction(row.name, 'restart');
        }
      },
    },
    {
      label: tc('batch.down'),
      icon: IconArrowDown,
      iconClassName: 'text-chart-2',
      variant: 'default',
      confirm: true,
      onClick: (rows) => {
        for (const row of rows as StackInfo[]) {
          if (isProtectedStack(row)) continue;
          onAction(row.name, 'down');
        }
      },
    },
    {
      label: tc('batch.remove'),
      icon: IconTrash,
      iconClassName: 'text-destructive',
      variant: 'destructive',
      confirm: true,
      onClick: (rows) => {
        for (const row of rows as StackInfo[]) {
          if (isProtectedStack(row)) continue;
          onDelete(row.name);
        }
      },
    },
  ];
}
