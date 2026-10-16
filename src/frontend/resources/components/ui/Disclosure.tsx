// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState, type ReactNode } from 'react';
import { IconChevronDown } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';

type DisclosureProps = {
  /** Title rendered inside the clickable summary. */
  title: ReactNode;
  /** Optional secondary content rendered to the right of the title (e.g. a badge). */
  trailing?: ReactNode;
  /** Optional small icon rendered before the title. */
  icon?: ReactNode;
  /** Description text rendered under the title in the summary row. */
  description?: ReactNode;
  /** Body rendered when the section is expanded. */
  children: ReactNode;
  /** Initial open state. Defaults to `false` (collapsed). */
  defaultOpen?: boolean;
  /** Controlled open state — pair with `onOpenChange` for fully controlled usage. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
  bodyClassName?: string;
  ariaLabel?: string;
  /** Disable interaction entirely (header renders but no toggle). */
  disabled?: boolean;
};

/**
 * Disclosure is a small collapsible section with a chevron that rotates
 * on expand. It pairs well with `<Section>` to provide an opt-in
 * "advanced settings" panel inside a form. Pass `defaultOpen` for
 * uncontrolled usage or `open` + `onOpenChange` for full control.
 *
 * Accessibility:
 *   - The summary is a real <PlainButton> with aria-expanded.
 *   - The body has a matching `id` and the button has aria-controls.
 *   - When `disabled` is set, the button is rendered with aria-disabled.
 */
export function Disclosure({
  title,
  trailing,
  icon,
  description,
  children,
  defaultOpen = false,
  open: isOpenX,
  onOpenChange,
  className,
  bodyClassName,
  ariaLabel,
  disabled = false,
}: DisclosureProps) {
  const isControlled = isOpenX !== undefined;
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const isOpen = isControlled ? Boolean(isOpenX) : internalOpen;
  const reactId = useId();
  const panelId = `${reactId}-panel`;

  function toggle() {
    if (disabled) return;
    const next = !isOpen;
    if (!isControlled) setInternalOpen(next);
    onOpenChange?.(next);
  }

  return (
    <div className={cn('mc-glass overflow-hidden rounded-lg', disabled && 'opacity-60', className)}>
      <PlainButton
        type="button"
        onClick={toggle}
        aria-expanded={isOpen}
        aria-controls={panelId}
        aria-disabled={disabled || undefined}
        aria-label={ariaLabel}
        disabled={disabled}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background focus-visible:outline-none disabled:cursor-not-allowed"
      >
        <IconChevronDown
          className={cn('size-4 shrink-0 text-muted-foreground transition-transform', isOpen && 'rotate-180')}
          aria-hidden="true"
        />
        {icon && <span className="shrink-0 text-muted-foreground">{icon}</span>}
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">{title}</span>
          {description && <span className="mt-0.5 block truncate text-xs text-muted-foreground">{description}</span>}
        </span>
        {trailing && <span className="shrink-0">{trailing}</span>}
      </PlainButton>
      {isOpen && (
        <section
          id={panelId}
          aria-label={typeof title === 'string' ? title : undefined}
          className={cn('space-y-3 border-t border-border px-4 py-3', bodyClassName)}
        >
          {children}
        </section>
      )}
    </div>
  );
}
