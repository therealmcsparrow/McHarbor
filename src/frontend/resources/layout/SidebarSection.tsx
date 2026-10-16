// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { IconChevronDown } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import type { NavSection } from './sidebar-data';
import { SidebarLink } from './SidebarLink';
import { SidebarSectionMenu } from './SidebarSectionMenu';
import type { SidebarItemBadgeData } from './SidebarItemBadge';

// Section header is a real button so keyboard users can collapse /
// expand with Enter or Space. The chevron rotates to telegraph state
// and we announce expanded / collapsed via aria-expanded.
type SidebarSectionProps = {
  section: NavSection;
  sectionCollapsed: boolean;
  sidebarCollapsed: boolean;
  onToggleSection: (id: string) => void;
  linkClass: (props: { isActive: boolean }) => string;
  /** `to` of the item that owns the current route, or null. */
  activePath: string | null;
  badges: Record<string, SidebarItemBadgeData>;
  t: (key: string) => string;
};

export function SidebarSection({
  section,
  sectionCollapsed,
  sidebarCollapsed,
  onToggleSection,
  linkClass,
  activePath,
  badges,
  t,
}: SidebarSectionProps) {
  const showHeader = !!section.labelKey && !sidebarCollapsed;
  // A header-less section (the dashboard sentinel) has nothing to click,
  // so it can never be folded away.
  const showItems = !showHeader || !sectionCollapsed;
  const headerLabel = section.labelKey ? t(section.labelKey) : '';

  // Collapsed rail: a labelled group is represented by its own icon, and
  // its items move into a popup carrying the group name as a header. The
  // dashboard sentinel has no label and no group identity, so it keeps
  // rendering as the single icon link it already is.
  if (sidebarCollapsed && section.labelKey) {
    if (!section.items.length) return null;
    return <SidebarSectionMenu section={section} label={headerLabel} activePath={activePath} itemLabel={t} />;
  }

  return (
    <div>
      {showHeader && (
        <PlainButton
          type="button"
          onClick={() => onToggleSection(section.id)}
          aria-expanded={!sectionCollapsed}
          aria-controls={`sidebar-section-${section.id}`}
          aria-label={
            sectionCollapsed
              ? `${t('nav.expandSection')}: ${headerLabel}`
              : `${t('nav.collapseSection')}: ${headerLabel}`
          }
          className={cn(
            'group mt-3 flex w-full items-center gap-2 px-4 pt-1 pb-2 text-[11px] font-semibold tracking-[0.18em] uppercase transition-colors',
            sectionCollapsed
              ? 'text-muted-foreground/70 hover:text-foreground'
              : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <span
            aria-hidden="true"
            className={cn('h-2.5 w-0.5 shrink-0', sectionCollapsed ? 'bg-border' : 'bg-primary')}
          />
          <span className="flex-1 truncate text-left">{headerLabel}</span>
          <IconChevronDown
            className={cn(
              'size-3.5 shrink-0 transition-transform duration-150',
              sectionCollapsed ? '-rotate-90' : 'rotate-0',
            )}
          />
        </PlainButton>
      )}
      {showItems && (
        <ul id={`sidebar-section-${section.id}`} className={cn(sidebarCollapsed && 'space-y-1')}>
          {section.items.map((item) => (
            <li key={item.to}>
              <SidebarLink
                item={item}
                collapsed={sidebarCollapsed}
                linkClass={linkClass}
                isActive={item.to === activePath}
                label={t(item.labelKey)}
                badge={badges[item.to]}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
