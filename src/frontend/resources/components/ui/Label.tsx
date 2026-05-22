// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type ComponentPropsWithoutRef, type ElementRef } from 'react';
import * as LabelPrimitive from '@radix-ui/react-label';
import { cn } from '@resources/utils/cn';

const Label = forwardRef<
  ElementRef<typeof LabelPrimitive.Root>,
  ComponentPropsWithoutRef<typeof LabelPrimitive.Root>
>(({ className, ...props }, ref) => (
  <LabelPrimitive.Root
    ref={ref}
    className={cn('block text-sm font-medium text-foreground', className)}
    {...props}
  />
));
Label.displayName = 'Label';

export { Label };
