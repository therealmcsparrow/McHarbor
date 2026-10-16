// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { IconFolder } from '@tabler/icons-react';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import type { NavSection } from './sidebar-data';

type SidebarSectionMenuProps = {
  section: NavSection;
  /** Already permission-filtered and translated by the caller. */
  label: string;
  /** `to` of the item that owns the current route, or null. */
  activePath: string | null;
  itemLabel: (labelKey: string) => string;
};

/**
 * A whole nav group, folded down to the one icon that fits the collapsed
 * rail. Clicking it opens the group's items in a popup headed by the group
 * name — the header is what replaces the section title the rail no longer
 * has room to print, so the reader still knows which group they opened.
 *
 * Badges stay out of this menu on purpose: the collapsed rail drops them
 * everywhere (see SidebarLink), and a count inside a transient popup is
 * not a status indicator anyone can watch.
 */
export function SidebarSectionMenu({ section, label, activePath, itemLabel }: SidebarSectionMenuProps) {
  const navigate = useNavigate();
  const GroupIcon = section.icon ?? IconFolder;
  const ownsActive = section.items.some((item) => item.to === activePath);

  return (
    <div className="mt-1 flex justify-center">
      <DropdownMenu.Root>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenu.Trigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={label}
                className={cn(
                  'size-9 rounded-lg',
                  ownsActive ? 'bg-primary/10 text-primary' : 'text-primary/60 hover:bg-primary/10 hover:text-primary',
                )}
              >
                <GroupIcon className="size-5 shrink-0" />
              </Button>
            </DropdownMenu.Trigger>
          </TooltipTrigger>
          <TooltipContent side="right" sideOffset={8}>
            {label}
          </TooltipContent>
        </Tooltip>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="right"
            align="start"
            sideOffset={8}
            className="mc-glass-overlay mc-menu z-50 min-w-[12rem] overflow-hidden rounded-lg p-1 text-popover-foreground"
          >
            <DropdownMenu.Label className="px-2 py-1.5 text-xs font-semibold tracking-wider text-muted-foreground uppercase">
              {label}
            </DropdownMenu.Label>
            <DropdownMenu.Separator className="my-1 h-px bg-border" />
            {section.items.map((item) => {
              const isActive = item.to === activePath;
              return (
                <DropdownMenu.Item
                  key={item.to}
                  // Radix owns focus and dismissal, so navigation goes through
                  // the router imperatively rather than nesting a NavLink.
                  onSelect={() => navigate(item.to)}
                  className={cn(
                    'flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm transition-colors outline-none focus:bg-accent focus:text-accent-foreground',
                    isActive && 'bg-primary/10 text-primary',
                  )}
                >
                  <item.icon className="size-4 shrink-0" aria-hidden="true" />
                  <span className="truncate">{itemLabel(item.labelKey)}</span>
                </DropdownMenu.Item>
              );
            })}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}
