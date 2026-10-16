// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createContext, forwardRef, useContext, type ComponentPropsWithoutRef, type ElementRef } from 'react';
import * as TooltipPrimitive from '@radix-ui/react-tooltip';
import { cn } from '@resources/utils/cn';

const TooltipProvider = TooltipPrimitive.Provider;
const Tooltip = TooltipPrimitive.Root;

/**
 * True for anything rendered as a tooltip trigger's child.
 *
 * `Button` reads it to decide whether to supply its own tooltip: an icon button
 * a caller has already wrapped must not wrap itself again, because a Radix
 * tooltip nested inside another one's trigger opens neither.
 */
const InsideTooltipTrigger = createContext(false);

export function useInsideTooltipTrigger(): boolean {
  return useContext(InsideTooltipTrigger);
}

const TooltipTrigger = forwardRef<
  ElementRef<typeof TooltipPrimitive.Trigger>,
  ComponentPropsWithoutRef<typeof TooltipPrimitive.Trigger>
>(({ children, ...props }, ref) => (
  // The provider wraps the trigger rather than sitting inside it. With `asChild`
  // Radix clones the trigger's single child to attach its handlers, and a context
  // provider is not a DOM element — nesting it inside makes Radix clone the
  // provider instead of the button, and then no tooltip in the app opens at all.
  <InsideTooltipTrigger.Provider value={true}>
    <TooltipPrimitive.Trigger ref={ref} {...props}>
      {children}
    </TooltipPrimitive.Trigger>
  </InsideTooltipTrigger.Provider>
));
TooltipTrigger.displayName = TooltipPrimitive.Trigger.displayName;

const TooltipContent = forwardRef<
  ElementRef<typeof TooltipPrimitive.Content>,
  ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 4, ...props }, ref) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn('mc-tooltip z-[200] rounded-lg px-3 py-1.5 text-xs text-popover-foreground', className)}
      {...props}
    />
  </TooltipPrimitive.Portal>
));
TooltipContent.displayName = TooltipPrimitive.Content.displayName;

export { TooltipProvider, Tooltip, TooltipTrigger, TooltipContent };
