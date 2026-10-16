// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What is this account, and what can it reach?
//
// Two cards: the facts stored on the row, and a count of everything the
// other five tabs own. The counts are links, not decoration — the point of
// an overview is that it hands you off to the tab that answers the
// question it just raised.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Button } from '@resources/components/ui/Button';
import { formatDate, timeAgo } from '@resources/utils/format';
import { useAnyPermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { useUserGroups, useUserRoles, useUserOverrides, type UserItem } from '@resources/hooks/useUsers';
import { useUserSessions } from '../../hooks/useAccountManagement';
import { userAccountState } from '../user-status';
import type { UserDetailTabId } from '../user-detail-tabs';

type UserOverviewTabProps = {
  user: UserItem;
  onNavigate: (tab: UserDetailTabId) => void;
};

export function UserOverviewTab({ user, onNavigate }: UserOverviewTabProps) {
  const { t } = useTranslation('security');
  const { data: groups = [] } = useUserGroups(user.id);
  const { data: roles = [] } = useUserRoles(user.id);
  const { data: overrides = [] } = useUserOverrides(user.id);
  // The admin session endpoint needs `sessions.manage`, and it polls every
  // 30s. Asking for it without the grant is a 403 on repeat, so the count
  // is simply not offered to a caller who cannot read it.
  const canViewSessions = useAnyPermission([PERM.SESSIONS_VIEW, PERM.SESSIONS_MANAGE]);
  const { data: sessions = [] } = useUserSessions(canViewSessions ? user.id : undefined);
  const state = userAccountState(user);

  const facts: { label: string; value: string }[] = [
    { label: t('users.username'), value: user.username },
    { label: t('users.displayName'), value: user.displayName || '—' },
    { label: t('users.email'), value: user.email || '—' },
    { label: t('users.role'), value: user.role },
    {
      label: t('users.lastLogin'),
      value: user.lastLogin ? timeAgo(user.lastLogin) : t('users.never'),
    },
    { label: t('users.createdAt'), value: formatDate(user.createdAt) },
  ];

  const access: { tab: UserDetailTabId; label: string; count: number }[] = [
    { tab: 'groups', label: t('users.detail.tabs.groups'), count: groups.length },
    { tab: 'roles', label: t('users.detail.tabs.roles'), count: roles.length },
    {
      tab: 'permissions',
      label: t('users.detail.tabs.permissions'),
      count: overrides.length,
    },
  ];
  if (canViewSessions) {
    access.push({
      tab: 'sessions',
      label: t('users.detail.tabs.sessions'),
      count: sessions.length,
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {state !== 'active' && (
        <Card className="border-destructive/40">
          <CardContent className="flex items-start gap-3">
            <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" aria-hidden="true" />
            <div>
              <p className="text-sm font-medium text-foreground">
                {state === 'disabled' ? t('users.detail.disabledTitle') : t('users.detail.lockedTitle')}
              </p>
              <p className="mt-1 text-sm text-muted-foreground">
                {state === 'disabled'
                  ? t('users.detail.disabledDescription')
                  : t('users.detail.lockedDescription', {
                      until: user.lockedUntil ? formatDate(user.lockedUntil) : '—',
                    })}
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('users.detail.accountTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
              {facts.map((fact) => (
                <div key={fact.label} className="min-w-0">
                  <dt className="text-xs text-muted-foreground">{fact.label}</dt>
                  <dd className="mt-0.5 truncate text-sm text-foreground">{fact.value}</dd>
                </div>
              ))}
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('users.detail.accessTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2">
              {access.map((entry) => (
                <li key={entry.tab}>
                  <Button variant="ghost" className="w-full justify-between px-3" onClick={() => onNavigate(entry.tab)}>
                    <span className="text-sm">{entry.label}</span>
                    <span className="text-sm font-semibold tabular-nums">{entry.count}</span>
                  </Button>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
