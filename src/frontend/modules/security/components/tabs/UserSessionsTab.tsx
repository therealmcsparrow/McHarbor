// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Where this account is currently signed in.
//
// It was the bottom half of the account panel in the users rail, where the
// list of every device a user has open competed with four other lists for
// a third of the page. It is a list; it gets a tab.

import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { RequirePermission } from '@core/auth/RequirePermission';
import { PERM } from '@core/auth/permissionKeys';
import { SessionsManager } from '@resources/components/SessionsManager';

type UserSessionsTabProps = {
  userId: string;
};

export function UserSessionsTab({ userId }: UserSessionsTabProps) {
  const { t } = useTranslation('security');

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('account.sessions')}</CardTitle>
      </CardHeader>
      <CardContent>
        <RequirePermission
          permission={[PERM.SESSIONS_VIEW, PERM.SESSIONS_MANAGE]}
          fallback={
            <TypographyText size="xs" tone="muted">
              {t('account.noSessionPermission')}
            </TypographyText>
          }
        >
          <SessionsManager userId={userId} />
        </RequirePermission>
      </CardContent>
    </Card>
  );
}
