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
    <div className={cn('flex items-center justify-between rounded-lg border border-border bg-card p-4', className)}>
      <div className="flex items-center gap-3 min-w-0">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium text-foreground truncate">{title}</p>
            {badge}
          </div>
          {subtitle && <p className="mt-0.5 text-xs text-muted-foreground truncate">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="flex items-center gap-1 shrink-0 ml-4">{actions}</div>}
    </div>
  );
}
