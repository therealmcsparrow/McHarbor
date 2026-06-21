// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { createPortal } from 'react-dom';
import { NavLink } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  IconLayoutDashboard,
  IconBox,
  IconPhoto,
  IconDeviceFloppy,
  IconNetwork,
  IconStack2,
  IconTerminal,
  IconFileText,
  IconWorld,
  IconBook,
  IconGitBranch,
  IconActivity,
  IconClipboardList,
  IconSettings,
  IconRefresh,
  IconApps,
  IconGitMerge,
  IconCube,
  IconRocket,
  IconServer,
  IconFolders,
  IconShieldLock,
  IconBrandDocker,
  IconBell,
} from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useSidebarStore } from '@resources/stores/sidebar';

type NavItem = {
  to: string;
  labelKey: string;
  icon: typeof IconLayoutDashboard;
};

const commonBefore: NavItem[] = [
  { to: '/dashboard', labelKey: 'nav.dashboard', icon: IconLayoutDashboard },
];

const dockerItems: NavItem[] = [
  { to: '/containers', labelKey: 'nav.containers', icon: IconBox },
  { to: '/images', labelKey: 'nav.images', icon: IconPhoto },
  { to: '/volumes', labelKey: 'nav.volumes', icon: IconDeviceFloppy },
  { to: '/stacks', labelKey: 'nav.stacks', icon: IconStack2 },
  { to: '/networks', labelKey: 'nav.networks', icon: IconNetwork },
  { to: '/environments', labelKey: 'nav.environments', icon: IconWorld },
  { to: '/terminal', labelKey: 'nav.terminal', icon: IconTerminal },
  { to: '/logs', labelKey: 'nav.logs', icon: IconFileText },
  { to: '/activity', labelKey: 'nav.activity', icon: IconActivity },
  { to: '/audit', labelKey: 'nav.audit', icon: IconClipboardList },
  { to: '/reconciler', labelKey: 'nav.reconciler', icon: IconRefresh },
  { to: '/workflows', labelKey: 'nav.workflows', icon: IconGitMerge },
  { to: '/store', labelKey: 'nav.store', icon: IconApps },
  { to: '/blueprints', labelKey: 'nav.blueprints', icon: IconBook },
  { to: '/git', labelKey: 'nav.git', icon: IconGitBranch },
];

const k8sItems: NavItem[] = [
  { to: '/pods', labelKey: 'nav.pods', icon: IconCube },
  { to: '/deployments', labelKey: 'nav.deployments', icon: IconRocket },
  { to: '/k8s-services', labelKey: 'nav.services', icon: IconServer },
  { to: '/namespaces', labelKey: 'nav.namespaces', icon: IconFolders },
];

const commonAfter: NavItem[] = [
  { to: '/environments', labelKey: 'nav.environments', icon: IconWorld },
  { to: '/activity', labelKey: 'nav.activity', icon: IconActivity },
  { to: '/audit', labelKey: 'nav.audit', icon: IconClipboardList },
  { to: '/reconciler', labelKey: 'nav.reconciler', icon: IconRefresh },
  { to: '/workflows', labelKey: 'nav.workflows', icon: IconGitMerge },
  { to: '/store', labelKey: 'nav.store', icon: IconApps },
  { to: '/blueprints', labelKey: 'nav.blueprints', icon: IconBook },
  { to: '/git', labelKey: 'nav.git', icon: IconGitBranch },
];

function SidebarLink({
  item,
  collapsed,
  linkClass,
  label,
}: {
  item: NavItem;
  collapsed: boolean;
  linkClass: (props: { isActive: boolean }) => string;
  label: string;
}) {
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
        end={item.to === '/dashboard'}
        className={linkClass}
        onMouseEnter={handleEnter}
        onMouseLeave={() => setTip(null)}
      >
        <item.icon className="size-4 shrink-0" />
        {!collapsed && label}
      </NavLink>
      {tip &&
        createPortal(
          <div
            className="pointer-events-none fixed z-50 whitespace-nowrap rounded-md border border-border bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md"
            style={{ top: tip.top, left: tip.left, transform: 'translateY(-50%)' }}
          >
            {label}
          </div>,
          document.body
        )}
    </>
  );
}

export function Sidebar() {
  const { t } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);
  const currentId = useEnvironmentStore((s) => s.currentId);
  const currentEnv = environments.find((e) => e.id === currentId);
  const isK8s = currentEnv?.orchestratorType === 'kubernetes';
  const collapsed = useSidebarStore((s) => s.collapsed);

  const navItems = isK8s
    ? [...commonBefore, ...k8sItems, ...commonAfter]
    : [...commonBefore, ...dockerItems];

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    collapsed
      ? cn(
          'flex items-center justify-center py-1.5 rounded-lg transition-colors',
          isActive
            ? 'bg-primary/10 text-primary'
            : 'text-primary/60 hover:text-primary hover:bg-primary/10'
        )
      : cn(
          'flex items-center gap-x-3.5 py-1 px-2.5 text-sm rounded-lg transition-colors',
          isActive
            ? 'bg-primary/10 text-primary font-medium'
            : 'text-muted-foreground hover:bg-muted/50'
        );

  const bottomItems: NavItem[] = [
    ...(!isK8s ? [{ to: '/docker', labelKey: 'nav.docker', icon: IconBrandDocker }] : []),
    { to: '/security', labelKey: 'nav.security', icon: IconShieldLock },
    { to: '/settings', labelKey: 'nav.settings', icon: IconSettings },
    { to: '/notifications', labelKey: 'nav.notifications', icon: IconBell },
  ];

  return (
    <aside
      className={cn(
        'flex h-full flex-col border-r border-sidebar-border bg-card transition-all duration-200',
        collapsed ? 'w-14' : 'w-64'
      )}
    >
      <div
        className={cn(
          'flex h-14 items-center gap-2 border-b border-sidebar-border',
          collapsed ? 'justify-center px-0' : 'px-5'
        )}
      >
        <img src="/icon_McSparrow.svg" alt="McHarbor" className="h-6 w-6" />
        {!collapsed && (
          <span className="text-lg font-bold text-sidebar-foreground">McHarbor</span>
        )}
      </div>

      <nav className={cn('flex-1 overflow-y-auto py-4', collapsed ? 'px-1.5' : 'px-3')}>
        <ul className="space-y-1">
          {navItems.map((item) => (
            <li key={item.to}>
              <SidebarLink
                item={item}
                collapsed={collapsed}
                linkClass={linkClass}
                label={t(item.labelKey)}
              />
            </li>
          ))}
        </ul>
      </nav>

      {/* Docker + Security + Settings at bottom */}
      <div className={cn('shrink-0', collapsed ? 'px-1.5' : 'px-3')}>
        <div className="border-t border-sidebar-border" />
        <div className="space-y-0.5 py-2">
          {bottomItems.map((item) => (
            <SidebarLink
              key={item.to}
              item={item}
              collapsed={collapsed}
              linkClass={linkClass}
              label={t(item.labelKey)}
            />
          ))}
        </div>
      </div>
    </aside>
  );
}

