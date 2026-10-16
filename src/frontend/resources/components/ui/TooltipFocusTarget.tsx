// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createElement, forwardRef, type HTMLAttributes } from 'react';
import { cn } from '@resources/utils/cn';

/**
 * Focusable wrapper for a disabled control so its explanatory tooltip still opens.
 * A disabled button fires no pointer or focus events, so the wrapper carries the tab stop.
 */
export const TooltipFocusTarget = forwardRef<HTMLSpanElement, HTMLAttributes<HTMLSpanElement>>(
  function TooltipFocusTargetBase({ className, ...props }, ref) {
    return createElement('span', { ref, tabIndex: 0, className: cn('inline-flex', className), ...props });
  },
);

TooltipFocusTarget.displayName = 'TooltipFocusTarget';
