// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What this key is allowed to do.
//
// Two questions, in the order they get asked. First where access comes from
// — which roles are attached, and which environments the key may touch.
// Then the permissions themselves, as one searchable list rather than the
// four chip walls this tab replaced.

import { useTranslation } from 'react-i18next';
import { IconWorld } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { APIKeyPermissionMatrix } from '../APIKeyPermissionMatrix';
import type { APIKeyDetail } from '../../hooks/useAPIKeys';

type APIKeyAccessTabProps = {
  detail: APIKeyDetail;
};

export function APIKeyAccessTab({ detail }: APIKeyAccessTabProps) {
  const { t } = useTranslation('security');

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('apiKeys.detail.assignedRoles')}</CardTitle>
          </CardHeader>
          <CardContent>
            {detail.roleDetails.length === 0 ? (
              <TypographyText size="sm" tone="muted">
                {t('apiKeys.detail.noRoles')}
              </TypographyText>
            ) : (
              // No nested permission list here on purpose: the matrix below
              // already names the granting role on every row, so repeating
              // each role's keys would be the same wall of chips twice.
              <ul className="divide-y divide-border">
                {detail.roleDetails.map((role) => (
                  <li key={role.id} className="flex items-start justify-between gap-4 py-2 first:pt-0 last:pb-0">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-foreground">{role.name}</p>
                      {role.description && (
                        <TypographyText size="xs" tone="muted" className="mt-0.5">
                          {role.description}
                        </TypographyText>
                      )}
                    </div>
                    <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                      {t('apiKeys.grantsCount', { count: role.permissions.length })}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('apiKeys.environments')}</CardTitle>
          </CardHeader>
          <CardContent>
            {detail.environments.length === 0 ? (
              <p className="flex items-center gap-2 text-sm text-muted-foreground">
                <IconWorld className="size-4" aria-hidden="true" />
                {t('apiKeys.allEnvironments')}
              </p>
            ) : (
              <ul className="flex flex-wrap gap-1.5">
                {detail.environments.map((environment) => (
                  <li key={environment.id}>
                    <Badge variant="secondary">{environment.name}</Badge>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">
            {detail.effectiveSnapshot ? t('apiKeys.detail.effectiveAtDeletion') : t('apiKeys.detail.permissionsTitle')}
          </CardTitle>
          <TypographyText size="xs" tone="muted" className="mt-1">
            {detail.effectiveSnapshot ? t('apiKeys.detail.effectiveSnapshotHint') : t('apiKeys.detail.permissionsHint')}
          </TypographyText>
        </CardHeader>
        <CardContent>
          <APIKeyPermissionMatrix detail={detail} />
        </CardContent>
      </Card>
    </div>
  );
}
