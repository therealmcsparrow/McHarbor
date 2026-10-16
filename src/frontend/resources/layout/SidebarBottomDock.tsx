// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconDotsVertical } from '@tabler/icons-react';
import * as DropdownMenu from '@radix-ui/react-dropdown-menu';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';
import type { NavItem } from './sidebar-data';
import { BottomDockLink } from './BottomDockLink';
import type { SidebarItemBadgeData } from './SidebarItemBadge';

type SidebarBottomDockProps = {
  items: NavItem[];
  /** `to` of the item that owns the current route, or null. */
  activePath: string | null;
  badges: Record<string, SidebarItemBadgeData>;
  collapsed: boolean;
};

/**
 * The app-level dock that sits under the nav. Expanded, it is a row of
 * icon buttons. Collapsed, the rail is only 14 units wide — four icons
 * side by side would either overflow or shrink past a comfortable hit
 * target — so the row folds into a single vertical-dots button whose
 * menu carries the same four destinations, this time with their labels.
 */
export function SidebarBottomDock({ items, activePath, badges, collapsed }: SidebarBottomDockProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();

  if (!collapsed) {
    return (
      <ul className="flex items-center justify-evenly py-1">
        {items.map((item) => (
          <li key={item.to}>
            <BottomDockLink
              item={item}
              label={t(item.labelKey)}
              isActive={item.to === activePath}
              badge={badges[item.to]}
            />
          </li>
        ))}
      </ul>
    );
  }

  return (
    <div className="flex items-center justify-center py-1">
      <DropdownMenu.Root>
        <Tooltip>
          <TooltipTrigger asChild>
            <DropdownMenu.Trigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t('nav.more')}
                className={cn(
                  'size-9 rounded-none',
                  activePath && items.some((item) => item.to === activePath)
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                <IconDotsVertical className="size-5 shrink-0" />
              </Button>
            </DropdownMenu.Trigger>
          </TooltipTrigger>
          <TooltipContent side="right" sideOffset={8}>
            {t('nav.more')}
          </TooltipContent>
        </Tooltip>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="right"
            align="end"
            sideOffset={8}
            className="mc-glass-overlay mc-menu z-50 min-w-[12rem] overflow-hidden rounded-lg p-1 text-popover-foreground"
          >
            {items.map((item) => {
              const isActive = item.to === activePath;
              return (
                <DropdownMenu.Item
                  key={item.to}
                  // Radix owns focus and dismissal here, so navigation goes
                  // through the router imperatively rather than nesting a
                  // NavLink inside the item.
                  onSelect={() => navigate(item.to)}
                  className={cn(
                    'flex cursor-pointer items-center gap-2 px-2 py-1.5 text-sm transition-colors outline-none focus:bg-accent focus:text-accent-foreground',
                    isActive && 'bg-primary/10 text-primary',
                  )}
                >
                  <item.icon className="size-4 shrink-0" aria-hidden="true" />
                  <span className="truncate">{t(item.labelKey)}</span>
                </DropdownMenu.Item>
              );
            })}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  );
}
