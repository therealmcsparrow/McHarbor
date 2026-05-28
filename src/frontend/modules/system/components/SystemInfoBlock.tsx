// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import type { TablerIcon } from '@tabler/icons-react';

export function SystemInfoBlock({
  title,
  description,
  icon: Icon,
  children,
}: {
  title: string;
  description?: string;
  icon: TablerIcon;
  children: ReactNode;
}) {
  return (
    <section className="rounded-lg border border-border bg-muted/20 p-4">
      <div className="mb-4 flex items-start gap-3">
        <div className="rounded-md bg-primary/10 p-2 text-primary">
          <Icon className="size-4" />
        </div>
        <div>
          <h2 className="text-sm font-semibold text-foreground">{title}</h2>
          {description && (
            <p className="mt-1 text-sm text-muted-foreground">{description}</p>
          )}
        </div>
      </div>
      <div className="divide-y divide-border">{children}</div>
    </section>
  );
}

export function SystemInfoRow({
  label,
  value,
}: {
  label: string;
  value: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="max-w-[65%] break-words text-right text-sm font-medium text-foreground">
        {value}
      </span>
    </div>
  );
}
