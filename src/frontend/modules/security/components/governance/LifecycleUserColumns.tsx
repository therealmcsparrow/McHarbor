// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Hint } from '@resources/components/ui/Hint';
import { formatDate, timeAgo } from '@resources/utils/format';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { dateColumn, textColumn } from '@resources/layout/table-page/optional-columns';
import type { LifecycleUser } from '../../hooks/useUserLifecycle';

type BadgeVariant = 'secondary' | 'warning' | 'outline' | 'info' | 'destructive';

type LifecycleBadge = { key: string; variant: BadgeVariant };

export function lifecycleBadges(user: LifecycleUser): LifecycleBadge[] {
  const badges: LifecycleBadge[] = [];
  if (user.disabled) badges.push({ key: 'disabled', variant: 'secondary' });
  if (user.locked) badges.push({ key: 'locked', variant: 'warning' });
  if (!user.mfaEnabled) badges.push({ key: 'noMfa', variant: 'outline' });
  if (user.inactive) badges.push({ key: 'inactive', variant: 'outline' });
  if (user.recentlyElevated) badges.push({ key: 'recentlyElevated', variant: 'info' });
  if (user.breakGlass) badges.push({ key: 'breakGlass', variant: 'destructive' });
  return badges;
}

export function lifecycleSourceLabel(source: string, t: TFunction<'security'>): string {
  return source === 'local' ? t('governance.lifecycle.sources.local') : source;
}

export function useLifecycleUserColumns(): ColumnDef<LifecycleUser, unknown>[] {
  const { t } = useTranslation('security');

  return useMemo<ColumnDef<LifecycleUser, unknown>[]>(
    () => [
      {
        id: 'user',
        accessorFn: (user) => user.displayName || user.username,
        header: t('governance.lifecycle.columns.user'),
        meta: { width: 'lg' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-medium text-foreground">{row.original.displayName || row.original.username}</span>
        ),
      },
      {
        id: 'state',
        accessorFn: (user) =>
          lifecycleBadges(user)
            .map((badge) => t(`governance.lifecycle.badges.${badge.key}`))
            .join(', '),
        header: t('governance.lifecycle.columns.state'),
        meta: { width: 'xl', wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex flex-wrap gap-1">
            {lifecycleBadges(row.original).map((badge) => (
              <Badge key={badge.key} variant={badge.variant}>
                {t(`governance.lifecycle.badges.${badge.key}`)}
              </Badge>
            ))}
          </div>
        ),
      },
      {
        id: 'source',
        accessorFn: (user) => lifecycleSourceLabel(user.authProvider, t),
        header: t('governance.lifecycle.columns.source'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{lifecycleSourceLabel(row.original.authProvider, t)}</span>
        ),
      },
      {
        id: 'lastLogin',
        accessorFn: (user) => user.lastLogin ?? '',
        sortFn: 'datetime',
        header: t('governance.lifecycle.columns.lastLogin'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.lastLogin ? (
            <Hint text={formatDate(row.original.lastLogin)}>
              <span className="text-xs text-muted-foreground">{timeAgo(row.original.lastLogin)}</span>
            </Hint>
          ) : (
            <span className="text-xs text-muted-foreground">{t('governance.lifecycle.never')}</span>
          ),
      },
      textColumn<LifecycleUser>('username', t('columns.usernameRaw'), (user) => user.username),
      dateColumn<LifecycleUser>(
        'lastElevatedAt',
        t('governance.lifecycle.columns.lastElevated'),
        (user) => user.lastElevatedAt,
      ),
      dateColumn<LifecycleUser>('createdAt', t('columns.createdAt'), (user) => user.createdAt),
    ],
    [t],
  );
}
