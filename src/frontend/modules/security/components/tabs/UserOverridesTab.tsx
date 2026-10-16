// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Per-user permission overrides.
//
// Reads need `users.view`; writes need `rbac.policies.manage` on the
// backend and `permissions.assign` on each control. The panel itself is
// gated on the read permission — a user who cannot read the override list
// gets an explanation rather than an empty form that will 403 on load.

import { useTranslation } from 'react-i18next';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { RequirePermission } from '@core/auth/RequirePermission';
import { PERM } from '@core/auth/permissionKeys';
import { UserOverridesEditor } from '../UserOverridesEditor';

type UserOverridesTabProps = {
  userId: string;
  userDisplayName: string;
};

export function UserOverridesTab({ userId, userDisplayName }: UserOverridesTabProps) {
  const { t } = useTranslation('security');

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('users.overrides.title')}</CardTitle>
        <TypographyText size="xs" tone="muted" className="mt-1">
          {t('users.overrides.description', { user: userDisplayName })}
        </TypographyText>
      </CardHeader>
      <CardContent>
        <RequirePermission
          permission={PERM.USERS_VIEW}
          fallback={
            <TypographyText size="xs" tone="muted">
              {t('users.detail.noOverridePermission')}
            </TypographyText>
          }
        >
          <UserOverridesEditor userId={userId} />
        </RequirePermission>
      </CardContent>
    </Card>
  );
}
