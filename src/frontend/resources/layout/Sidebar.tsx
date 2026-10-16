// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router';
import { IconBrandDocker, IconBell, IconShieldLock, IconSettings } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { useSidebarStore } from '@resources/stores/sidebar';
import { useVisibleNavItems } from '@resources/hooks/useVisibleNavItems';
import { usePluginMenu } from '@core/plugins/loader';
import {
  buildSections,
  dashboardSection,
  deliverSection,
  pluginMenuToSections,
  type NavItem,
  type NavSection,
} from '@resources/layout/sidebar-data';
import { resolveActiveNavPath } from './nav-active';
import { SidebarSection } from './SidebarSection';
import { SidebarBottomDock } from './SidebarBottomDock';
import { SidebarEnvironmentBadge } from './SidebarEnvironmentBadge';
import { useSidebarStatusBadges } from './useSidebarStatusBadges';
import { useBrandLogo } from '@resources/hooks/useBranding';

const EMPTY_PLUGIN_MENU: NonNullable<ReturnType<typeof usePluginMenu>> = [];

export function Sidebar() {
  const { t } = useTranslation('common');
  const { pathname } = useLocation();
  const collapsed = useSidebarStore((s) => s.collapsed);
  const expandedSection = useSidebarStore((s) => s.expandedSection);
  const toggleSection = useSidebarStore((s) => s.toggleSection);
  const setExpandedSection = useSidebarStore((s) => s.setExpandedSection);
  // The install's own mark for the active variant, falling back to the
  // bundled McHarbor one. Both surfaces resolve independently, so an operator
  // who uploaded only a wordmark keeps the shipped icon on the collapsed rail.
  const wordmark = useBrandLogo('wide');
  const icon = useBrandLogo('icon');

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    collapsed
      ? cn(
          'flex items-center justify-center border-l-2 py-2 transition-colors',
          isActive
            ? 'border-primary bg-primary/10 text-primary'
            : 'border-transparent text-primary/60 hover:bg-accent hover:text-primary',
        )
      : cn(
          'flex items-center justify-between gap-x-3.5 border-l-2 py-2 pr-4 pl-3.5 text-sm transition-colors',
          isActive
            ? 'border-primary bg-primary/10 font-semibold text-primary'
            : 'border-transparent text-sidebar-foreground hover:bg-accent',
        );

  // Build the full section list once per (mode, plugin menu snapshot).
  // Dashboard stays label-less so its header is skipped by the renderer.
  const pluginMenu = usePluginMenu() ?? EMPTY_PLUGIN_MENU;
  const sections: NavSection[] = useMemo(() => {
    return [
      dashboardSection,
      ...buildSections('docker'),
      // Plugins that claim a section id get their own named section;
      // everything else lands in the generic "Plugins" section.
      ...pluginMenuToSections(pluginMenu),
      // Deliver closes the sidebar. It is appended here rather than
      // returned by `buildSections` because "last" has to mean last:
      // inside that list it would sit above every plugin-owned section,
      // and installing a plugin would silently move it up the list.
      deliverSection,
    ];
  }, [pluginMenu]);

  // Pre-compute per-section visible items. We need to satisfy rules of
  // hooks, so we run `useVisibleNavItems` once on the flat list and
  // group the surviving items back into sections for rendering.
  const allVisibleItems = useVisibleNavItems(sections.flatMap((s) => s.items));
  const sectionsWithVisible = useMemo(() => {
    const byTo = new Map(allVisibleItems.map((it) => [it.to, it]));
    return (
      sections
        .map((s) => ({
          ...s,
          items: s.items.map((it) => byTo.get(it.to)).filter((it): it is NavItem => Boolean(it)),
        }))
        // A section whose every item was permission-filtered has nothing
        // left to navigate to, and a header that opens onto an empty list
        // reads as a broken page rather than as an absent permission. Drop
        // it entirely: the section is a grouping of destinations, not a
        // destination itself, so with no destinations it has no meaning.
        .filter((s) => s.items.length > 0)
    );
  }, [sections, allVisibleItems]);

  // Bottom dock stays outside the scroll area; it doesn't fold with
  // sections and isn't permission-gated beyond `useVisibleNavItems`.
  //
  // Four app-level destinations, and nothing here that isn't a page — the
  // light/dark control moved out to Settings → Theme, where the rest of the
  // appearance preferences already live. `/security` (access & roles)
  // resolves to this dock rather than to the Govern section, so it still
  // appears exactly once in the nav; Govern keeps the scans page.
  const bottomItems: NavItem[] = useVisibleNavItems([
    { to: '/docker', labelKey: 'nav.docker', icon: IconBrandDocker },
    { to: '/notifications', labelKey: 'nav.notifications', icon: IconBell },
    { to: '/security', labelKey: 'nav.security', icon: IconShieldLock },
    { to: '/settings', labelKey: 'nav.settings', icon: IconSettings },
  ]);

  const visibleItems = useMemo(
    () => [...sectionsWithVisible.flatMap((section) => section.items), ...bottomItems],
    [bottomItems, sectionsWithVisible],
  );
  const visiblePaths = useMemo(() => visibleItems.map((item) => item.to), [visibleItems]);
  const badges = useSidebarStatusBadges(visiblePaths);

  // One winner for the whole nav. Resolving centrally (longest match wins)
  // is what stops a parent route — /plugins/addon-kubernetes under its tabs, or the
  // reporting plugin's /plugins/addon-reporting under its own sub-pages — from
  // staying lit once the reader has moved on to a sibling.
  const activePath = useMemo(() => resolveActiveNavPath(pathname, visibleItems), [pathname, visibleItems]);

  // Sections are an accordion: one open at a time. Navigating into a folded
  // section opens it, but only on an actual route change, so folding the
  // section you are standing in stays folded until you navigate again.
  const lastPathRef = useRef<string | null>(null);
  useEffect(() => {
    if (lastPathRef.current === pathname) return;
    lastPathRef.current = pathname;
    const owner = sectionsWithVisible.find(
      (section) => section.labelKey && section.items.some((item) => item.to === activePath),
    );
    if (owner) setExpandedSection(owner.id);
  }, [activePath, pathname, sectionsWithVisible, setExpandedSection]);

  return (
    <aside
      className={cn(
        'relative z-10 flex h-full flex-col border-r border-border bg-sidebar transition-all duration-200',
        collapsed ? 'w-14' : 'w-64',
      )}
    >
      <div
        className={cn(
          'flex h-14 items-center justify-center gap-2 border-b border-border',
          collapsed ? 'px-0' : 'px-5',
        )}
      >
        {/* The wordmark when there is room for it; the icon on the 56px rail,
            where a 3.4:1 lockup cannot fit. Same `alt` either way — the sidebar
            should not appear to rename itself when it collapses. The aspect
            ratio is pinned because the source SVG declares width/height 100%,
            which a height-only rule can resolve to zero width. */}
        {collapsed ? (
          <span className="grid size-[30px] shrink-0 place-items-center">
            <img
              src={icon.src}
              alt={t('brand.logoAlt')}
              className={cn(
                'h-[17px] w-[17px]',
                // An uploaded mark is an unknown aspect ratio, so it is
                // contained in the box rather than stretched to it.
                icon.isCustom && 'size-auto max-h-[17px] max-w-[17px] object-contain',
              )}
            />
          </span>
        ) : (
          <img
            src={wordmark.src}
            alt={t('brand.logoAlt')}
            className={cn(
              'h-7 w-auto shrink-0',
              // The bundled SVG declares width/height 100%, which a
              // height-only rule can resolve to zero width — hence the pinned
              // ratio. An uploaded file carries its own intrinsic size and
              // must not inherit it.
              wordmark.isCustom ? 'max-w-[13rem] object-contain' : '[aspect-ratio:7038/2094]',
            )}
          />
        )}
      </div>

      <div className={cn('shrink-0 border-b border-border', collapsed ? 'flex justify-center py-1.5' : '')}>
        <SidebarEnvironmentBadge />
      </div>

      <nav className="flex-1 overflow-y-auto py-2">
        {sectionsWithVisible.map((section) => {
          const sectionCollapsed = section.id !== expandedSection;
          return (
            <SidebarSection
              key={section.id}
              section={section}
              sectionCollapsed={sectionCollapsed}
              sidebarCollapsed={collapsed}
              onToggleSection={toggleSection}
              linkClass={linkClass}
              activePath={activePath}
              badges={badges}
              t={t}
            />
          );
        })}
      </nav>

      {/* Bottom dock: app-level pages that don't fit a thematic section.
       * Mobile-app style: horizontal row of icon-only buttons, folding
       * into a single vertical-dots menu once the rail is collapsed —
       * see SidebarBottomDock. */}
      <div className="shrink-0">
        <div className="border-t border-border" />
        <SidebarBottomDock items={bottomItems} activePath={activePath} badges={badges} collapsed={collapsed} />
      </div>
    </aside>
  );
}
