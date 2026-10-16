// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType, ReactNode } from 'react';
import { Badge } from '@resources/components/ui/Badge';

type IconComponent = ComponentType<{ className?: string }>;

type SectionHeroProps = {
  icon: IconComponent;
  title: string;
  badgeIcon: IconComponent;
  badgeLabel: string;
  description: string;
  actions?: ReactNode;
};

export function SectionHero({
  icon: Icon,
  title,
  badgeIcon: BadgeIcon,
  badgeLabel,
  description,
  actions,
}: SectionHeroProps) {
  return (
    <div className="flex flex-1 flex-col gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className="grid size-10 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
          >
            <Icon className="size-6 text-primary" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-xl leading-tight font-semibold tracking-tight text-foreground">
              {title}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              <Badge variant="secondary" className="gap-1 px-1.5 py-0 text-[10px]">
                <BadgeIcon className="size-3" />
                {badgeLabel}
              </Badge>
              <span className="min-w-0 text-xs text-muted-foreground">{description}</span>
            </div>
          </div>
        </div>

        {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
      </div>
    </div>
  );
}
