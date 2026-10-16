// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createElement, forwardRef, type ButtonHTMLAttributes } from 'react';
import { cn } from '@resources/utils/cn';

export type PlainButtonProps = ButtonHTMLAttributes<HTMLButtonElement>;

/**
 * Unstyled button primitive for chips, tabs, tree rows, swatches and layout
 * chrome whose look is owned entirely by the caller.
 *
 * It only guarantees `type="button"` (no accidental form submits) and a
 * visible keyboard focus ring; everything else comes from `className`. Reach
 * for `<Button>` when the element should look like an action button.
 */
export const PlainButton = forwardRef<HTMLButtonElement, PlainButtonProps>(function PlainButtonBase(
  { className, type = 'button', ...props },
  ref,
) {
  return createElement('button', {
    ref,
    type,
    className: cn('focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none', className),
    ...props,
  });
});

PlainButton.displayName = 'PlainButton';
