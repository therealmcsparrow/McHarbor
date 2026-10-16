// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { cssVars } from '@resources/utils/cssVars';
import { useState } from 'react';
import { createPortal } from 'react-dom';
import { NavLink } from 'react-router';
import type { NavItem } from './sidebar-data';
import { SidebarItemBadge, type SidebarItemBadgeData } from './SidebarItemBadge';

type SidebarLinkProps = {
  item: NavItem;
  collapsed: boolean;
  linkClass: (props: { isActive: boolean }) => string;
  label: string;
  /**
   * Resolved by `resolveActiveNavPath` across the whole nav, not by NavLink.
   * NavLink's own rule is a plain prefix test, which keeps a parent route lit
   * on all of its children — `/plugins/addon-kubernetes` on every cluster page,
   * `/plugins/addon-reporting` on every reporting page. Longest match wins instead,
   * and only the winner is told it is active.
   */
  isActive: boolean;
  badge?: SidebarItemBadgeData;
};

export function SidebarLink({ item, collapsed, linkClass, label, isActive, badge }: SidebarLinkProps) {
  const [tip, setTip] = useState<{ top: number; left: number } | null>(null);

  function handleEnter(e: React.MouseEvent<HTMLAnchorElement>) {
    if (!collapsed) return;
    const rect = e.currentTarget.getBoundingClientRect();
    setTip({ top: rect.top + rect.height / 2, left: rect.right + 12 });
  }

  return (
    <>
      <NavLink
        to={item.to}
        className={linkClass({ isActive })}
        onMouseEnter={handleEnter}
        onMouseLeave={() => setTip(null)}
      >
        <span className="flex min-w-0 items-center gap-x-3.5">
          <item.icon className="size-4 shrink-0" />
          {!collapsed && <span className="truncate">{label}</span>}
        </span>
        {/* Badges are dropped while the rail is collapsed — a bare count
         * floating over an icon reads as noise without the label it
         * belongs to, and the tooltip already carries the destination. */}
        {badge && !collapsed && <SidebarItemBadge badge={badge} collapsed={collapsed} />}
      </NavLink>
      {tip &&
        createPortal(
          <div
            className="pointer-events-none fixed top-(--y) left-(--x) z-50 -translate-y-1/2 border border-border bg-popover px-3 py-1.5 text-xs whitespace-nowrap text-popover-foreground"
            style={cssVars({ x: `${tip.left}px`, y: `${tip.top}px` })}
          >
            {label}
          </div>,
          document.body,
        )}
    </>
  );
}
