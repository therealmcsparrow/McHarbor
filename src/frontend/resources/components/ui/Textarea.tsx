// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type TextareaHTMLAttributes } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@resources/utils/cn';

const textareaVariants = cva(
  'w-full rounded-lg border text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary disabled:opacity-50 disabled:pointer-events-none min-h-[80px]',
  {
    variants: {
      variant: {
        default: 'py-2.5 px-4 bg-card border-border',
        outline: 'py-2 px-3 bg-background border-input focus:outline-none focus:ring-2 focus:ring-ring',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
);

type TextareaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & VariantProps<typeof textareaVariants>;

const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, variant, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(textareaVariants({ variant, className }))}
      {...props}
    />
  )
);
Textarea.displayName = 'Textarea';

export { Textarea, textareaVariants };
