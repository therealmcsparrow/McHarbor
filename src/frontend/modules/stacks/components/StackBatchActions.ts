// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconPlayerStop,
  IconRotate,
  IconArrowDown,
  IconArrowUp,
  IconTrash,
} from '@tabler/icons-react';
import type { BatchAction } from '@resources/components/DataGrid';
import { isProtectedStack } from '@core/utils/protection';
import type { StackInfo } from '../hooks/useStacks';

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
      variant: 'default',
      onClick: (rows) => onUpdateSelected?.(rows as StackInfo[]),
    },
    {
      label: tc('batch.reinstallSelected'),
      icon: IconRotate,
      variant: 'default',
      onClick: (rows) => onReinstallSelected?.(rows as StackInfo[]),
    },
    {
      label: tc('batch.stop'),
      icon: IconPlayerStop,
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
