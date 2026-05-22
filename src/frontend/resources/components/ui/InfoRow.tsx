// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

type InfoRowProps = {
  label: string;
  children: ReactNode;
  className?: string;
};

export function InfoRow({ label, children, className }: InfoRowProps) {
  return (
    <div className={cn('flex gap-4 border-b border-border py-2 last:border-0', className)}>
      <span className="w-36 shrink-0 text-sm font-medium text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words text-sm text-foreground">{children}</span>
    </div>
  );
}
