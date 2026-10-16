// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Level 1 of the container detail navigation: the five groups. Split out of
// ContainerDetailTabs.tsx.

import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import type { DetailGroupId } from './detail-tab-groups';
import { groupIcons } from './detail-tab-registry';

type DetailGroupRailProps = {
  groups: DetailGroupId[];
  activeGroup: DetailGroupId;
  onSelectGroup: (group: DetailGroupId) => void;
  groupLabel: (group: DetailGroupId) => string;
  ariaLabel: string;
};

export function DetailGroupRail({ groups, activeGroup, onSelectGroup, groupLabel, ariaLabel }: DetailGroupRailProps) {
  return (
    <div className="flex items-center gap-2">
      <div role="tablist" aria-label={ariaLabel} className="flex flex-wrap items-center gap-1">
        {groups.map((group) => {
          const Icon = groupIcons[group];
          const active = group === activeGroup;
          return (
            <Button
              key={group}
              role="tab"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              variant={active ? 'secondary' : 'ghost'}
              size="sm"
              className={cn('gap-1.5', active ? 'font-semibold text-foreground' : 'font-normal text-muted-foreground')}
              onClick={() => onSelectGroup(group)}
            >
              <Icon className="size-4" />
              {groupLabel(group)}
            </Button>
          );
        })}
      </div>
    </div>
  );
}
