// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Label } from './Label';
import { cn } from '@resources/utils/cn';

type FormFieldProps = {
  label: string;
  htmlFor?: string;
  description?: string;
  className?: string;
  children: ReactNode;
};

export function FormField({ label, htmlFor, description, className, children }: FormFieldProps) {
  return (
    <div className={cn('space-y-1.5', className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
    </div>
  );
}
