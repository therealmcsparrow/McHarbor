// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { NavLink } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconBell, IconDeviceMobile, IconServer2, IconShieldCheck, type Icon } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';

const TABS: Array<{ to: string; key: 'alerts' | 'fleet' | 'approvals' | 'device'; icon: Icon }> = [
  { to: '/oncall/alerts', key: 'alerts', icon: IconBell },
  { to: '/oncall/fleet', key: 'fleet', icon: IconServer2 },
  { to: '/oncall/approvals', key: 'approvals', icon: IconShieldCheck },
  { to: '/oncall/device', key: 'device', icon: IconDeviceMobile },
];

export function BottomBar() {
  const { t } = useTranslation('oncall');
  return (
    <nav
      aria-label={t('nav.label')}
      className="shrink-0 border-t border-border bg-card pb-[env(safe-area-inset-bottom)]"
    >
      <ul className="mx-auto grid max-w-xl grid-cols-4">
        {TABS.map((tab) => (
          <li key={tab.key}>
            <NavLink
              to={tab.to}
              className={({ isActive }) =>
                cn(
                  'flex min-h-14 flex-col items-center justify-center gap-0.5 text-xs font-medium',
                  isActive ? 'text-primary' : 'text-muted-foreground',
                )
              }
            >
              <tab.icon className="size-6" aria-hidden="true" />
              <span>{t(`nav.${tab.key}`)}</span>
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
