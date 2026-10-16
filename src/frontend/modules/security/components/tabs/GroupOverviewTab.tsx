// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What is this group, and what does it hand out?
//
// Two cards, like the user overview: the facts stored on the row, and a
// count of everything the other two tabs own. The counts are links — the
// point of an overview is that it hands you off to the tab that answers the
// question it just raised.

import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Button } from '@resources/components/ui/Button';
import { formatDate } from '@resources/utils/format';
import type { Group } from '../../hooks/useGroups';
import type { GroupDetailTabId } from '../group-detail-tabs';

type GroupOverviewTabProps = {
  group: Group;
  onNavigate: (tab: GroupDetailTabId) => void;
};

export function GroupOverviewTab({ group, onNavigate }: GroupOverviewTabProps) {
  const { t } = useTranslation('security');
  const memberCount = group.members?.length ?? group.memberCount;
  const roleCount = group.roles?.length ?? 0;

  const facts: { label: string; value: string }[] = [
    { label: t('groups.name'), value: group.name },
    { label: t('groups.groupDescription'), value: group.description || '—' },
    {
      label: t('groups.type'),
      value: group.isSystem ? t('groups.system') : t('groups.custom'),
    },
    { label: t('groups.createdAt'), value: formatDate(group.createdAt) },
    { label: t('groups.updatedAt'), value: formatDate(group.updatedAt) },
  ];

  const access: { tab: GroupDetailTabId; label: string; count: number }[] = [
    { tab: 'members', label: t('groups.detail.tabs.members'), count: memberCount },
    { tab: 'roles', label: t('groups.detail.tabs.roles'), count: roleCount },
  ];

  return (
    <div className="flex flex-col gap-4">
      {group.isSystem && (
        <Card>
          <CardContent className="flex items-start gap-3">
            <IconLock className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
            <div>
              <p className="text-sm font-medium text-foreground">{t('groups.detail.systemTitle')}</p>
              <p className="mt-1 text-sm text-muted-foreground">{t('groups.systemGroupLocked')}</p>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('groups.detail.detailsTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
              {facts.map((fact) => (
                <div key={fact.label} className="min-w-0">
                  <dt className="text-xs text-muted-foreground">{fact.label}</dt>
                  <dd className="mt-0.5 truncate text-sm text-foreground">{fact.value}</dd>
                </div>
              ))}
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('groups.detail.accessTitle')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2">
              {access.map((entry) => (
                <li key={entry.tab}>
                  <Button variant="ghost" className="w-full justify-between px-3" onClick={() => onNavigate(entry.tab)}>
                    <span className="text-sm">{entry.label}</span>
                    <span className="text-sm font-semibold tabular-nums">{entry.count}</span>
                  </Button>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
