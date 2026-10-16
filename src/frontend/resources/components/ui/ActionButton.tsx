// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Button } from './Button';
import { Tooltip, TooltipTrigger, TooltipContent } from './Tooltip';
import { TooltipFocusTarget } from './TooltipFocusTarget';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';

/**
 * Icon-only button with a tooltip, for dense table and card action rows.
 *
 * Lives here because containers, stacks and the Kubernetes plugin all need it — it was
 * duplicated in the first two (the stacks copy had drifted, missing
 * `className`) until the third consumer arrived.
 *
 * Two things about the disabled state are worth knowing before editing this.
 *
 * First, `Button` sets `disabled:pointer-events-none`, so a disabled button
 * opens no tooltip — and in a row of six identical glyphs the tooltip is the
 * only thing that says which one this is, let alone why it is dead. A disabled
 * button is therefore wrapped in a focusable span, which keeps the label
 * reachable by pointer and by keyboard. (HeaderActionButton carries the same
 * fix; this copy had been missing it, so every `disabled` ActionButton in the
 * app was a mute grey glyph.)
 *
 * Second, `permission` is handled here rather than forwarded to `Button`.
 * Forwarding would nest one Radix Tooltip inside another's trigger; instead
 * the missing-permission reason is appended to the label this button already
 * shows, so the user gets one tooltip that says both what the control is and
 * why they cannot use it.
 */
type ActionButtonProps = {
  label: string;
  onClick: () => void;
  icon: React.ReactNode;
  className?: string;
  disabled?: boolean;
  /** Permission key, or any-of list, required to use this action. */
  permission?: string | string[];
};

export function ActionButton({ label, onClick, icon, className, disabled = false, permission }: ActionButtonProps) {
  const { denied, reason } = usePermissionGate(permission);
  const isDisabled = disabled || denied;
  const tooltip = denied ? `${label} — ${reason}` : label;

  const button = (
    <Button
      variant="ghost"
      size="icon-sm"
      className={className}
      aria-label={tooltip}
      disabled={isDisabled}
      onClick={(evt) => {
        evt.stopPropagation();
        if (isDisabled) return;
        onClick();
      }}
    >
      {icon}
    </Button>
  );

  return (
    <Tooltip>
      <TooltipTrigger asChild>{isDisabled ? <TooltipFocusTarget>{button}</TooltipFocusTarget> : button}</TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
