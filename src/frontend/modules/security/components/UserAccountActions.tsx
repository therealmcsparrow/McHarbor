// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconKey, IconLock, IconLockOpen, IconShieldOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Flex } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useEnableAccount, useUnlockAccount } from '../hooks/useAccountManagement';
import { ResetPasswordDialog } from './ResetPasswordDialog';
import { DisableAccountDialog } from './DisableAccountDialog';
import { isUserLockedOut } from './user-status';
import type { UserItem } from '@resources/hooks/useUsers';
import { PERM } from '@core/auth/permissionKeys';

type UserAccountActionsProps = {
  user: UserItem;
};

/**
 * UserAccountActions renders the per-user admin actions — reset password,
 * disable/enable, clear lockout — in the Account tab of the user detail
 * page. Every action is gated on a permission so the UI cleanly hides
 * options the caller cannot perform.
 *
 * Sessions used to hang off the bottom of this component; they are their
 * own tab now, because a list of every device a user has open is not a
 * footnote to three buttons.
 */
export function UserAccountActions({ user }: UserAccountActionsProps) {
  const { t } = useTranslation('security');
  const [resetOpen, setResetOpen] = useState(false);
  const [disableOpen, setDisableOpen] = useState(false);
  const enable = useEnableAccount();
  const unlock = useUnlockAccount();
  const locked = isUserLockedOut(user);

  return (
    <div className="space-y-4">
      <div>
        <TypographyText size="sm" weight="medium" className="mb-2">
          {t('account.title')}
        </TypographyText>
        {/* Disabled with a tooltip, never hidden: an operator who cannot
            reset a password should still be able to see that resetting one
            is a thing this page does, and which grant would let them. */}
        <Flex wrap gap={2}>
          <Button
            permission={PERM.ACCOUNT_PASSWORD_RESET}
            variant="outline"
            size="sm"
            onClick={() => setResetOpen(true)}
          >
            <IconKey className="mr-1 size-3.5" />
            {t('account.resetPassword')}
          </Button>
          {user.isActive ? (
            <Button permission={PERM.ACCOUNT_MANAGE} variant="outline" size="sm" onClick={() => setDisableOpen(true)}>
              <IconLock className="mr-1 size-3.5" />
              {t('account.disable')}
            </Button>
          ) : (
            <Button
              permission={PERM.ACCOUNT_MANAGE}
              variant="outline"
              size="sm"
              onClick={() => enable.mutate(user.id)}
              disabled={enable.isPending}
            >
              <IconLockOpen className="mr-1 size-3.5" />
              {t('account.enable')}
            </Button>
          )}
          {!user.isActive && (
            <Flex align="center" gap={1} className="text-xs text-muted-foreground">
              <IconShieldOff className="size-3" />
              {t('account.disabled')}
            </Flex>
          )}
          <Button
            permission={PERM.ACCOUNT_MANAGE}
            variant="outline"
            size="sm"
            onClick={() => unlock.mutate(user.id)}
            disabled={!locked || unlock.isPending}
          >
            <IconLockOpen className="mr-1 size-3.5" />
            {t('account.clearLockout')}
          </Button>
        </Flex>
      </div>

      <ResetPasswordDialog open={resetOpen} onOpenChange={setResetOpen} userId={user.id} username={user.username} />
      <DisableAccountDialog
        open={disableOpen}
        onOpenChange={setDisableOpen}
        userId={user.id}
        username={user.username}
      />
    </div>
  );
}
