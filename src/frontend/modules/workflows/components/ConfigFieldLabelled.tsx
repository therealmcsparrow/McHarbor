// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { Label } from '@resources/components/ui/Label';

interface ConfigFieldLabelledProps {
  label: string;
  required?: boolean;
  children: ReactNode;
}

export function ConfigFieldLabelled({ label, required, children }: ConfigFieldLabelledProps) {
  return (
    <div>
      <Label className="mb-1.5 text-xs">
        {label}
        {required && <span className="text-destructive"> *</span>}
      </Label>
      {children}
    </div>
  );
}
