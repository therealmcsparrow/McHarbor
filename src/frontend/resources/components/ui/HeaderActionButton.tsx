// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// HeaderActionButton — an icon button in a detail hero's action row.
//
// The networks, stacks, images and volumes heroes each grew their own copy of
// this; they are now one component. The behaviour worth keeping from the
// networks copy: a disabled button fires no pointer events, so a tooltip on it
// would never open — and on these heroes the tooltip is the only place that
// explains *why* the button is dead ("built-in networks cannot be removed",
// "this stack is protected"). A disabled button is therefore wrapped in a
// focusable span, which keeps the explanation reachable by pointer and by
// keyboard.
//
// Dimming is left to `Button`'s own disabled styling rather than an extra
// opacity class, so every hero agrees on how a dead control looks.

import type { ReactNode } from 'react';
import { Button } from './Button';
import { Tooltip, TooltipTrigger, TooltipContent } from './Tooltip';
import { TooltipFocusTarget } from './TooltipFocusTarget';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';

type HeaderActionButtonProps = {
  /** Already translated. Doubles as the accessible name. */
  tooltip: string;
  onClick: () => void;
  icon: ReactNode;
  variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
  className?: string;
  disabled?: boolean;
  /**
   * Permission key, or any-of list, required to use this action.
   *
   * Handled here rather than forwarded to `Button` — forwarding would nest one
   * Radix Tooltip inside another's trigger. The reason is appended to the
   * tooltip this hero already shows, alongside the "built-in networks cannot
   * be removed" style explanations it carries for non-permission reasons.
   */
  permission?: string | string[];
};

export function HeaderActionButton({
  tooltip,
  onClick,
  icon,
  variant = 'outline',
  className,
  disabled = false,
  permission,
}: HeaderActionButtonProps) {
  const { denied, reason } = usePermissionGate(permission);
  const isDisabled = disabled || denied;
  const label = denied ? `${tooltip} — ${reason}` : tooltip;

  const button = (
    <Button
      variant={variant}
      size="icon-sm"
      aria-label={label}
      onClick={onClick}
      disabled={isDisabled}
      className={className}
    >
      {icon}
    </Button>
  );

  if (isDisabled) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <TooltipFocusTarget>{button}</TooltipFocusTarget>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
