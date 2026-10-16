// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { NavLink } from 'react-router';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import type { NavItem } from './sidebar-data';
import { SidebarItemBadge, type SidebarItemBadgeData } from './SidebarItemBadge';

type BottomDockLinkProps = {
  item: NavItem;
  label: string;
  /** Resolved centrally by `resolveActiveNavPath` — see SidebarLink. */
  isActive: boolean;
  badge?: SidebarItemBadgeData;
};

/**
 * BottomDockLink renders a single bottom-dock entry as a square icon
 * button. Always icon-only (no text label next to the icon) regardless
 * of sidebar collapsed/expanded state. The title is exposed via a
 * Radix tooltip on hover/focus, matching the mobile-app-style bottom
 * nav requested in the redesign.
 *
 * isActive is handed in rather than derived from NavLink so a dock entry
 * that is the prefix parent of a section route (`/security` vs the Govern
 * section's `/security/scans`) does not stay lit on its child's page.
 */
export function BottomDockLink({ item, label, isActive, badge }: BottomDockLinkProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <NavLink
          to={item.to}
          aria-label={label}
          className={cn(
            'relative flex h-9 w-9 items-center justify-center rounded-md transition-colors',
            isActive ? 'bg-primary/10 text-primary' : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
          )}
        >
          <item.icon className="size-5 shrink-0" />
          {badge && <SidebarItemBadge badge={badge} collapsed />}
        </NavLink>
      </TooltipTrigger>
      <TooltipContent side="right" sideOffset={8}>
        {label}
      </TooltipContent>
    </Tooltip>
  );
}
