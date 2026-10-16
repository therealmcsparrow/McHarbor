// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// UserDetailHero — the identity half of the user detail card.
//
// Same shape as `ContainerDetailHeader` and `SecuritySectionHero`: a tinted
// tile, an <h1> naming who you are looking at, a metadata row, and the
// destination's actions opposite. The tile carries initials rather than an
// icon, because every row in this section is a person and an identical
// person-glyph on all of them would identify nothing.
//
// The tile turns danger-toned when the account cannot sign in, so the state
// is legible before you read a single word.

import { useTranslation } from 'react-i18next';
import { IconKey, IconLockOpen, IconMail, IconUserCheck, IconUserOff } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { cn } from '@resources/utils/cn';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { useEnableAccount, useUnlockAccount } from '../hooks/useAccountManagement';
import { userAccountState } from './user-status';
import type { UserItem } from '@resources/hooks/useUsers';

type UserDetailHeroProps = {
  user: UserItem;
  onResetPassword: () => void;
  onDisable: () => void;
};

/** Up to two initials, from the display name when there is one. */
function initialsFor(user: UserItem): string {
  const source = (user.displayName || user.username).trim();
  const parts = source.split(/\s+/).filter(Boolean);
  const letters = parts.length > 1 ? `${parts[0]![0] ?? ''}${parts[parts.length - 1]![0] ?? ''}` : source.slice(0, 2);
  return letters.toUpperCase() || '?';
}

export function UserDetailHero({ user, onResetPassword, onDisable }: UserDetailHeroProps) {
  const { t } = useTranslation('security');
  const enable = useEnableAccount();
  const unlock = useUnlockAccount();
  const state = userAccountState(user);
  const itemName = user.displayName || user.username;

  return (
    <div className="flex flex-1 flex-col gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className={cn(
              'grid size-11 shrink-0 place-items-center rounded-xl text-sm font-semibold ring-1 ring-inset',
              state === 'active'
                ? 'bg-primary/12 text-primary ring-primary/25'
                : 'bg-destructive/12 text-destructive ring-destructive/25',
            )}
          >
            {initialsFor(user)}
          </span>
          <div className="min-w-0">
            <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
              {itemName}
            </h1>
            <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1">
              {state === 'disabled' && <Badge variant="secondary">{t('users.inactive')}</Badge>}
              {state === 'locked' && <Badge variant="destructive">{t('users.locked')}</Badge>}
              {state === 'active' && <Badge variant="success">{t('users.active')}</Badge>}
              <Badge variant="outline">{user.role}</Badge>
              {user.displayName && (
                <span className="truncate font-mono text-xs text-muted-foreground">@{user.username}</span>
              )}
              {user.email && (
                <span className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                  <IconMail className="size-3 shrink-0" aria-hidden="true" />
                  <span className="truncate">{user.email}</span>
                </span>
              )}
              <span className="text-xs text-muted-foreground">
                {t('users.lastLogin')}: {user.lastLogin ? timeAgo(user.lastLogin) : t('users.never')}
              </span>
            </div>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          <HeaderActionButton
            tooltip={t('account.resetPassword')}
            icon={<IconKey className="size-4" />}
            onClick={onResetPassword}
            permission={PERM.ACCOUNT_PASSWORD_RESET}
          />
          {user.isActive ? (
            <HeaderActionButton
              tooltip={t('account.disable')}
              icon={<IconUserOff className="size-4" />}
              onClick={onDisable}
              permission={PERM.ACCOUNT_MANAGE}
            />
          ) : (
            <HeaderActionButton
              tooltip={t('account.enable')}
              icon={<IconUserCheck className="size-4" />}
              onClick={() => enable.mutate(user.id)}
              disabled={enable.isPending}
              permission={PERM.ACCOUNT_MANAGE}
            />
          )}
          <HeaderActionButton
            tooltip={t('account.clearLockout')}
            icon={<IconLockOpen className="size-4" />}
            onClick={() => unlock.mutate(user.id)}
            disabled={state !== 'locked' || unlock.isPending}
            permission={PERM.ACCOUNT_MANAGE}
          />
        </div>
      </div>
    </div>
  );
}
