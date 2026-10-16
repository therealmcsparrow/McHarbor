// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type InputHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

// Surface, focus and hover come from `.mc-field` in app.css — the shared
// form-control material. Variants carry density only.
const inputVariants = cva(
  'mc-control mc-field w-full rounded-lg border text-sm text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:opacity-50',
  {
    // Height is fixed by `.mc-control`; variants carry horizontal density only.
    variants: {
      variant: {
        default: 'px-4',
        outline: 'px-3',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

type InputProps = InputHTMLAttributes<HTMLInputElement> & VariantProps<typeof inputVariants>;

const Input = forwardRef<HTMLInputElement, InputProps>(({ className, variant, ...props }, ref) => (
  <input ref={ref} className={cn(inputVariants({ variant, className }))} {...props} />
));
Input.displayName = 'Input';

export { Input, inputVariants };
