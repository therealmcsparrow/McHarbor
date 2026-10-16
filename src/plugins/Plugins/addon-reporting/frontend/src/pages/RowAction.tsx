// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { cn } from '@resources/utils/cn';

// One icon action in a table row, gated and honest about it.
//
// Why this exists rather than `<Tooltip><TooltipTrigger asChild><Button
// permission="…">`, which is what these columns used to do:
//
// When the caller lacks the permission, `Button` does not return a button.
// `GatedButton` returns its *own* `<Tooltip>` — a disabled button fires no
// pointer events, so it needs a focusable span to anchor the explanation.
// `Tooltip` is `TooltipPrimitive.Root`, so an outer `TooltipTrigger asChild`
// then hands its Slot a Radix Root instead of a DOM element: the merged props
// and the ref land on a component that forwards neither, the outer trigger has
// no anchor, and two nested Roots contend for one provider's open state. The
// "requires X" explanation never appears.
//
// That composition only works while the caller is allowed, which is exactly
// why it survived: every account it was tested with could use the buttons.
//
// So the gate is resolved here and `Button` is handed a plain `disabled`. It
// never receives `permission`, so the nested tooltip is never built and there
// is exactly one Tooltip per action — showing the label when the action is
// available and the missing permission when it is not.

type Props = {
  icon: ReactNode;
  /** What the action does. Shown as the tooltip when it is available. */
  label: string;
  /** Permission key, or any-of list, required to use it. */
  permission?: string | string[];
  /** A non-permission reason to block it — a run already in flight, say. */
  disabled?: boolean;
  /** Tints the glyph for destructive actions. */
  danger?: boolean;
  onClick: () => void;
};

export function RowAction({ icon, label, permission, disabled, danger, onClick }: Props) {
  const { denied, reason } = usePermissionGate(permission);
  const blocked = denied || Boolean(disabled);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* The span is always present, not only when blocked: a disabled
            button has `pointer-events-none`, so the trigger has to be
            something that still receives them. Keeping it in both states
            also keeps the trigger a stable DOM node across a permission
            change. It only takes a tab stop when the button cannot, so an
            available action is not two stops deep. */}
        <TooltipFocusTarget tabIndex={blocked ? 0 : -1} className={cn('rounded-lg', blocked && 'cursor-not-allowed')}>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            disabled={blocked}
            // The base variant's `disabled:opacity-50` is not a state change a
            // reader notices on an already-muted ghost glyph in a dense row.
            // A denied action is dimmer still, and the wrapper's cursor says
            // the same thing a second way.
            className={cn(denied && 'opacity-40', danger && 'text-danger')}
            onClick={(evt) => {
              // Rows are clickable in both tables this serves.
              evt.stopPropagation();
              onClick();
            }}
          >
            {icon}
          </Button>
        </TooltipFocusTarget>
      </TooltipTrigger>
      <TooltipContent>{denied ? reason : label}</TooltipContent>
    </Tooltip>
  );
}
