// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type HTMLAttributes, forwardRef } from 'react';
import { cn } from '@resources/utils/cn';

// VisuallyHidden renders content that is hidden from sighted users
// but still announced by screen readers. Use it for:
//   * supplementary context that's only meaningful to AT
//   * icon-only button labels (prefer `aria-label` first — this is
//     for inline text that should be skipped visually)
//   * clarifying data table column meaning (e.g. "before" / "after")
//
// The implementation uses Tailwind's `sr-only` utility, which is
// the standard accessible pattern: the element occupies a 1px box
// clipped to the viewport so it is still picked up by the AT tree.
export const VisuallyHidden = forwardRef<HTMLSpanElement, HTMLAttributes<HTMLSpanElement>>(function VisuallyHiddenBase(
  { className, ...props },
  ref,
) {
  return <span ref={ref} className={cn('sr-only', className)} {...props} />;
});

VisuallyHidden.displayName = 'VisuallyHidden';
