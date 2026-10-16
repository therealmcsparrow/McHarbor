// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

type ListItemProps = {
  title: string;
  subtitle?: string;
  badge?: ReactNode;
  actions?: ReactNode;
  className?: string;
};

export function ListItem({ title, subtitle, badge, actions, className }: ListItemProps) {
  return (
    <div className={cn('mc-glass flex items-center justify-between rounded-lg p-4', className)}>
      <div className="flex min-w-0 items-center gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="truncate text-sm font-medium text-foreground">{title}</p>
            {badge}
          </div>
          {subtitle && <p className="mt-0.5 truncate text-xs text-muted-foreground">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="ml-4 flex shrink-0 items-center gap-1">{actions}</div>}
    </div>
  );
}
