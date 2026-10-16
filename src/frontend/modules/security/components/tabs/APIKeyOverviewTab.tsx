// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// What is this credential, and where do I go next?
//
// Two cards, the same shape as `UserOverviewTab`: the facts stored on the
// row, and counts that hand the reader off to the tab answering the question
// they just raised. The counts are buttons, not decoration.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Button } from '@resources/components/ui/Button';
import { formatDate, timeAgo } from '@resources/utils/format';
import type { APIKeyDetail } from '../../hooks/useAPIKeys';
import type { APIKeyDetailTabId } from '../api-key-detail-tabs';

type APIKeyOverviewTabProps = {
  detail: APIKeyDetail;
  onNavigate: (tab: APIKeyDetailTabId) => void;
  /** Requests recorded in the graph's default window. */
  requestCount: number;
};

export function APIKeyOverviewTab({ detail, onNavigate, requestCount }: APIKeyOverviewTabProps) {
  const { t } = useTranslation('security');

  const facts: { label: string; value: string }[] = [
    { label: t('apiKeys.detail.owner'), value: detail.username },
    { label: t('apiKeys.key'), value: detail.keyPrefix },
    { label: t('apiKeys.status'), value: t(`apiKeys.${detail.status}`) },
    { label: t('apiKeys.detail.createdAt'), value: formatDate(detail.createdAt) },
    {
      label: t('apiKeys.lastUsed'),
      value: detail.lastUsedAt ? timeAgo(detail.lastUsedAt) : t('apiKeys.never'),
    },
    {
      label: t('apiKeys.expiresAt'),
      value: detail.expiresAt ? formatDate(detail.expiresAt) : t('apiKeys.noExpiry'),
    },
  ];
  if (detail.deletedAt) {
    facts.push({
      label: t('apiKeys.detail.deletedAt'),
      value: formatDate(detail.deletedAt),
    });
  }

  const links: { tab: APIKeyDetailTabId; label: string; count: number }[] = [
    { tab: 'access', label: t('apiKeys.roles'), count: detail.roleDetails.length },
    {
      tab: 'access',
      label: t('apiKeys.detail.effective'),
      count: detail.effectivePermissions.length,
    },
    {
      tab: 'access',
      label: t('apiKeys.environments'),
      count: detail.environments.length,
    },
    { tab: 'usage', label: t('apiKeys.detail.requests24h'), count: requestCount },
  ];

  return (
    <div className="flex flex-col gap-4">
      {detail.status === 'deleted' && (
        <Card className="border-warning/40">
          <CardContent className="flex items-start gap-3">
            <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" aria-hidden="true" />
            <div>
              <p className="text-sm font-medium text-foreground">{t('apiKeys.detail.deletedTitle')}</p>
              <p className="mt-1 text-sm text-muted-foreground">{t('apiKeys.detail.deletedBody')}</p>
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">{t('apiKeys.detail.factsTitle')}</CardTitle>
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
            <CardTitle className="text-sm">{t('apiKeys.detail.atAGlance')}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2">
              {links.map((entry) => (
                <li key={entry.label}>
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
