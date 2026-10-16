// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The admin actions, with their consequences spelled out.
//
// The hero carries the same three actions as icons, because an operator
// who already knows what they want should not have to find a tab first.
// This panel is for the operator who does not: each action gets a name, a
// sentence saying what it does to the account, and the current state it
// would act on.

import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { UserAccountActions } from '../UserAccountActions';
import { userAccountState } from '../user-status';
import type { UserItem } from '@resources/hooks/useUsers';

type UserAccountTabProps = {
  user: UserItem;
};

export function UserAccountTab({ user }: UserAccountTabProps) {
  const { t } = useTranslation('security');
  const state = userAccountState(user);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('account.title')}</CardTitle>
        <TypographyText size="xs" tone="muted" className="mt-1">
          {state === 'disabled'
            ? t('users.detail.disabledDescription')
            : state === 'locked'
              ? t('users.detail.lockedDescription', {
                  until: user.lockedUntil ? formatDate(user.lockedUntil) : '—',
                })
              : t('users.detail.accountHint')}
        </TypographyText>
      </CardHeader>
      <CardContent>
        <UserAccountActions user={user} />
      </CardContent>
    </Card>
  );
}
