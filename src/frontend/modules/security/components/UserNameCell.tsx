// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { cn } from '@resources/utils/cn';
import type { UserItem } from '@resources/hooks/useUsers';
import { userAccountState } from './user-status';

type UserNameCellProps = {
  user: UserItem;
};

export function UserNameCell({ user }: UserNameCellProps) {
  const { t } = useTranslation('security');
  const state = userAccountState(user);
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      {state !== 'active' && (
        <>
          <IconLock aria-hidden="true" className="size-3.5 shrink-0 text-destructive" />
          <span className="sr-only">{state === 'disabled' ? t('users.disabledAria') : t('users.lockedAria')}</span>
        </>
      )}
      <span className={cn('truncate font-medium', state === 'disabled' && 'text-destructive')}>
        {user.displayName || user.username}
      </span>
      {user.kind === 'service' && (
        <Badge variant="secondary" className="shrink-0">
          {t('users.servicePrincipal')}
        </Badge>
      )}
    </div>
  );
}
