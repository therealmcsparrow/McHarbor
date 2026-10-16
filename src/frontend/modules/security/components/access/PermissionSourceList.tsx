// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { PermissionSource } from '../../hooks/useAccessReview';

type Props = {
  sources: PermissionSource[];
  environmentNames: Record<string, string>;
};

function useSourceLabel(environmentNames: Record<string, string>) {
  const { t } = useTranslation('access');
  return (source: PermissionSource): string => {
    const scope = source.stackName
      ? t('effective.source.scopeStack', { stack: source.stackName })
      : source.environmentId
        ? t('effective.source.scopeEnvironment', {
            environment: environmentNames[source.environmentId] || source.environmentId,
          })
        : t('effective.source.scopeGlobal');
    switch (source.kind) {
      case 'role': {
        const base =
          source.via === 'group'
            ? t('effective.source.roleGroup', { role: source.roleName, group: source.groupName || source.groupId })
            : t('effective.source.roleDirect', { role: source.roleName });
        const wildcard = source.wildcard ? ` (${t('effective.source.wildcard')})` : '';
        return `${base}${wildcard}, ${scope}`;
      }
      case 'elevation':
        return t('effective.source.elevation', { until: source.until });
      case 'break_glass':
        return t('effective.source.breakGlass');
      default: {
        const base =
          source.kind === 'override_deny' ? t('effective.source.overrideDeny') : t('effective.source.overrideGrant');
        const parts = [base, scope];
        if (source.conditional) {
          parts.push(
            t('effective.source.conditional', {
              resource: [source.resourceType, source.resourceId].filter(Boolean).join(' '),
            }),
          );
        }
        if (source.until) parts.push(t('effective.source.expires', { until: source.until }));
        return parts.join(', ');
      }
    }
  };
}

export function PermissionSourceList({ sources, environmentNames }: Props) {
  const { t } = useTranslation('access');
  const label = useSourceLabel(environmentNames);
  if (sources.length === 0) {
    return <span className="text-xs text-muted-foreground">{t('effective.noSource')}</span>;
  }
  return (
    <ul className="flex flex-wrap gap-1">
      {sources.map((source) => {
        const text = label(source);
        return (
          <li
            key={`${source.kind}-${source.roleId ?? ''}-${source.groupId ?? ''}-${source.elevationId ?? ''}-${source.overrideId ?? ''}-${source.stackName ?? ''}`}
          >
            <Badge
              variant={source.kind === 'override_deny' ? 'destructive' : source.kind === 'role' ? 'outline' : 'info'}
            >
              {text}
            </Badge>
          </li>
        );
      })}
    </ul>
  );
}
