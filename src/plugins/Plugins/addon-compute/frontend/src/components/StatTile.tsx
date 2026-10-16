// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

/** The small bordered figure the environment cards use, shared by every card here. */
export function StatTile({
  label,
  value,
  icon,
  tone = 'default',
}: {
  label: string;
  value: ReactNode;
  icon?: ReactNode;
  tone?: 'default' | 'warning' | 'muted';
}) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 px-3 py-2">
      <div className="flex items-center gap-1.5 text-muted-foreground">
        {icon}
        <span className="truncate text-[11px] font-medium">{label}</span>
      </div>
      <p
        className={cn(
          'mt-1 truncate text-sm font-semibold tabular-nums',
          tone === 'warning' && 'text-warning',
          tone === 'muted' && 'text-muted-foreground',
          tone === 'default' && 'text-foreground',
        )}
      >
        {value}
      </p>
    </div>
  );
}
