// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ReactNode } from 'react';
import { useFocusTrap } from '@resources/hooks/useFocusTrap';
import { cn } from '@resources/utils/cn';

type FocusTrapProps = {
  active?: boolean;
  restoreFocus?: boolean;
  className?: string;
  children: ReactNode;
  /** When provided, the rendered element uses this tag instead of
   *  `<div>`. Useful when the trap needs to live inside a `<span>`
   *  or `<section>` for layout reasons. */
  as?: keyof React.JSX.IntrinsicElements;
};

// FocusTrap is a drop-in wrapper that traps keyboard focus inside
// its children while `active` is true. Use it for popovers,
// command palettes, side panels, and any other transient overlay
// where the underlying primitive does not already provide a trap.
//
// Radix's Dialog / Popover / Tooltip already trap focus correctly
// — only reach for this when you need a custom trap.
export function FocusTrap({
  active = true,
  restoreFocus = true,
  className,
  children,
  as: Tag = 'div',
}: FocusTrapProps) {
  const ref = useFocusTrap<HTMLDivElement>({ active, restoreFocus });
  const Wrapper = Tag as 'div';
  return (
    <Wrapper ref={ref as React.RefObject<HTMLDivElement>} className={cn(className)}>
      {children}
    </Wrapper>
  );
}
