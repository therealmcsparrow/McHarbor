// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The users table's action column.
//
// Three admin actions, in the order an operator reaches for them:
// reset the password, turn the account off (or back on), clear a lockout.
// Every one of them stops the click from bubbling — the row itself opens
// the user's detail page, and pressing "disable" should not also navigate.
//
// "Clear lockout" renders disabled rather than hidden when the account is
// not locked out, so the column keeps the same width on every row and the
// control stays discoverable.

import { useTranslation } from 'react-i18next';
import { IconKey, IconLockOpen, IconUserCheck, IconUserOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { useEnableAccount, useUnlockAccount } from '../hooks/useAccountManagement';
import { isUserLockedOut } from './user-status';
import type { UserItem } from '@resources/hooks/useUsers';

type UserRowActionsProps = {
  user: UserItem;
  /** Opens the reset-password dialog, which the table owns. */
  onResetPassword: (user: UserItem) => void;
  /** Opens the disable-account dialog, which asks for a reason. */
  onDisable: (user: UserItem) => void;
};

export function UserRowActions({ user, onResetPassword, onDisable }: UserRowActionsProps) {
  const { t } = useTranslation('security');
  const enable = useEnableAccount();
  const unlock = useUnlockAccount();
  const locked = isUserLockedOut(user);

  return (
    <div className="flex justify-end gap-1">
      <Button
        permission={PERM.ACCOUNT_PASSWORD_RESET}
        variant="ghost"
        size="icon-sm"
        aria-label={t('account.resetPassword')}
        onClick={(evt2) => {
          evt2.stopPropagation();
          onResetPassword(user);
        }}
      >
        <IconKey className="size-3.5" />
      </Button>

      {user.isActive ? (
        <Button
          permission={PERM.ACCOUNT_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('account.disable')}
          onClick={(entry) => {
            entry.stopPropagation();
            onDisable(user);
          }}
        >
          <IconUserOff className="size-3.5 text-destructive" />
        </Button>
      ) : (
        <Button
          permission={PERM.ACCOUNT_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('account.enable')}
          disabled={enable.isPending}
          onClick={(eventItem) => {
            eventItem.stopPropagation();
            enable.mutate(user.id);
          }}
        >
          <IconUserCheck className="size-3.5" />
        </Button>
      )}

      <Button
        permission={PERM.ACCOUNT_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('account.clearLockout')}
        disabled={!locked || unlock.isPending}
        onClick={(evt) => {
          evt.stopPropagation();
          unlock.mutate(user.id);
        }}
      >
        <IconLockOpen className="size-3.5" />
      </Button>
    </div>
  );
}
