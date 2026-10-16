// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// TableBulkActions — the buttons inside the bulk bar.
//
// The confirmation dialog for destructive actions already lived in
// DataGridBatchActions; this is the piece that was missing from that cluster —
// the icon buttons themselves. Actions marked `confirm: true` route through the
// dialog, everything else fires straight away.

import { useState } from 'react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { DataGridBatchActions } from '@resources/components/DataGridBatchActions';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { cn } from '@resources/utils/cn';
import type { BatchAction } from '@resources/layout/table-page/types';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';

type TableBulkActionsProps<T> = {
  actions: BatchAction[];
  selectedRows: T[];
  onComplete: () => void;
};

type TableBulkActionButtonProps = {
  action: BatchAction;
  onRun: (action: BatchAction) => void;
};

function TableBulkActionButton({ action, onRun }: TableBulkActionButtonProps) {
  const { denied, reason } = usePermissionGate(action.permission);
  const Icon = action.icon;
  const isDestructive = action.variant === 'destructive';
  const button = (
    <Button
      variant="ghost"
      size="icon-sm"
      className={cn(isDestructive && 'hover:bg-destructive/10 hover:text-destructive')}
      aria-label={action.label}
      disabled={denied}
      aria-disabled={denied || undefined}
      onClick={() => onRun(action)}
    >
      {Icon && (
        <Icon
          className={cn('h-4 w-4', action.iconClassName, isDestructive && !action.iconClassName && 'text-destructive')}
        />
      )}
    </Button>
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>{denied ? <TooltipFocusTarget>{button}</TooltipFocusTarget> : button}</TooltipTrigger>
      <TooltipContent>{denied ? reason : action.label}</TooltipContent>
    </Tooltip>
  );
}

export function TableBulkActions<T>({ actions, selectedRows, onComplete }: TableBulkActionsProps<T>) {
  const [pendingAction, setPendingAction] = useState<BatchAction | null>(null);

  const run = (action: BatchAction) => {
    if (action.confirm) {
      setPendingAction(action);
    } else {
      action.onClick(selectedRows);
    }
  };

  return (
    <>
      {actions.map((action) => (
        <TableBulkActionButton key={action.label} action={action} onRun={run} />
      ))}

      <DataGridBatchActions
        pendingAction={pendingAction}
        selectedCount={selectedRows.length}
        onCancel={() => setPendingAction(null)}
        onConfirm={() => {
          pendingAction?.onClick(selectedRows);
          setPendingAction(null);
          onComplete();
        }}
      />
    </>
  );
}
