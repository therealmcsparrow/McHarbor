// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { MappingPreview, RoleGrantView } from '../../hooks/useIdentitySetup';

type Impact = NonNullable<MappingPreview['impact']>;

export function RoleGrantBadge({
  grant,
  variant = 'outline',
}: {
  grant: RoleGrantView;
  variant?: 'outline' | 'success' | 'destructive';
}) {
  const { t } = useTranslation('access');
  const scope = grant.stackName
    ? t('preview.scopeStack', { stack: grant.stackName })
    : grant.environmentId
      ? t('preview.scopeEnvironment', { environment: grant.environmentId })
      : t('preview.scopeGlobal');
  return <Badge variant={variant}>{`${grant.roleName || grant.roleId} · ${scope}`}</Badge>;
}

function grantKey(grant: RoleGrantView) {
  return `${grant.roleId}-${grant.environmentId ?? ''}-${grant.stackName ?? ''}`;
}

export function RoleImpactList({ impact }: { impact: Impact }) {
  const { t } = useTranslation('access');
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-foreground">{t('impact.title')}</p>
      <p className="text-xs text-muted-foreground">
        {t('impact.summary', {
          evaluated: impact.evaluated,
          changed: impact.users.length,
          unchanged: impact.unchanged,
        })}
      </p>
      {impact.users.length > 0 && (
        <ul className="max-h-[40vh] space-y-2 overflow-y-auto" aria-label={t('impact.users')}>
          {impact.users.map((user) => (
            <li key={user.userId} className="rounded-md border border-border p-2 text-xs">
              <p className="font-medium text-foreground">{user.username}</p>
              <div className="mt-1 flex flex-wrap gap-1">
                {user.gained.map((grant) => (
                  <RoleGrantBadge key={`g-${grantKey(grant)}`} grant={grant} variant="success" />
                ))}
                {user.lost.map((grant) => (
                  <RoleGrantBadge key={`l-${grantKey(grant)}`} grant={grant} variant="destructive" />
                ))}
              </div>
              {(user.gained.length > 0 || user.lost.length > 0) && (
                <p className="mt-1 text-muted-foreground">
                  {t('impact.legend', {
                    count: user.gained.length,
                    gained: user.gained.length,
                    lost: user.lost.length,
                  })}
                </p>
              )}
              {user.joined.length > 0 && (
                <p className="text-muted-foreground">
                  {t('impact.joined', { groups: user.joined.map((g) => g.groupName).join(', ') })}
                </p>
              )}
              {user.left.length > 0 && (
                <p className="text-muted-foreground">
                  {t('impact.left', { groups: user.left.map((g) => g.groupName).join(', ') })}
                </p>
              )}
            </li>
          ))}
        </ul>
      )}
      {impact.unknown.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('impact.unknown', { users: impact.unknown.map((u) => u.username).join(', ') })}
        </p>
      )}
    </div>
  );
}
