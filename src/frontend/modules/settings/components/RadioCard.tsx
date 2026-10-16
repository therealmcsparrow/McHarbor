// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { cn } from '@resources/utils/cn';

type RadioCardProps = {
  /** Radios sharing a name form one group; use a per-picker `useId()`. */
  name: string;
  checked: boolean;
  onSelect: () => void;
  disabled?: boolean;
  permission?: string | string[];
  className?: string;
  children: ReactNode;
};

/**
 * A card-shaped native radio: the real input stays in the accessibility tree
 * (visually hidden) while the label draws the card, so keyboard and screen
 * reader behaviour is the browser's own rather than a re-implemented role.
 */
export function RadioCard({
  name: groupName,
  checked,
  onSelect,
  disabled,
  permission,
  className,
  children,
}: RadioCardProps) {
  const gate = usePermissionGate(permission);
  const isDisabled = Boolean(disabled) || gate.denied;

  return (
    <label
      title={gate.denied ? gate.reason : undefined}
      className={cn(
        'mc-glass relative flex cursor-pointer rounded-lg text-sm font-medium text-foreground transition-colors hover:bg-muted has-focus-visible:ring-2 has-focus-visible:ring-ring',
        checked && 'border-primary bg-primary/5',
        isDisabled && 'pointer-events-none cursor-not-allowed opacity-50',
        className,
      )}
    >
      <input
        type="radio"
        name={groupName}
        checked={checked}
        disabled={isDisabled}
        onChange={onSelect}
        className="peer sr-only"
      />
      {children}
    </label>
  );
}
