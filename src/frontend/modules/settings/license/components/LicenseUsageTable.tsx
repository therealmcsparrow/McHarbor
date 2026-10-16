// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChartHistogram } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { useLicenseUsage } from '../hooks/useLicense';
import { UNLIMITED_SERVERS } from '../types';

const PER_PAGE = 10;

export function LicenseUsageTable() {
  const { t } = useTranslation('license');
  const [page, setPage] = useState(1);
  const { data, isLoading } = useLicenseUsage(page, PER_PAGE);

  const items = data?.items ?? [];
  const totalPages = data?.total_pages ?? 1;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('usage.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <TypographyText tone="muted" size="xs" className="mb-3 block">
          {t('usage.description')}
        </TypographyText>

        {isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : items.length === 0 ? (
          <EmptyState icon={<IconChartHistogram className="size-6" />} title={t('usage.empty')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs text-muted-foreground">
                  <th className="py-2 pr-4 font-medium">{t('usage.sampledAt')}</th>
                  <th className="py-2 pr-4 font-medium">{t('usage.servers')}</th>
                  <th className="py-2 pr-4 font-medium">{t('usage.limit')}</th>
                  <th className="py-2 pr-4 font-medium">{t('usage.agents')}</th>
                  <th className="py-2 pr-4 font-medium">{t('usage.environments')}</th>
                  <th className="py-2 pr-4 font-medium">{t('usage.users')}</th>
                  <th className="py-2 font-medium">{t('usage.state')}</th>
                </tr>
              </thead>
              <tbody>
                {items.map((sample) => (
                  <tr key={sample.id} className="border-b border-border/50">
                    <td className="py-2 pr-4 whitespace-nowrap">{formatDate(sample.sampledAt)}</td>
                    <td className="py-2 pr-4">{sample.serversUsed}</td>
                    <td className="py-2 pr-4">
                      {sample.serversLimit === UNLIMITED_SERVERS ? '∞' : sample.serversLimit}
                    </td>
                    <td className="py-2 pr-4">{sample.agentCount}</td>
                    <td className="py-2 pr-4">{sample.envCount}</td>
                    <td className="py-2 pr-4">{sample.userCount}</td>
                    <td className="py-2">
                      <Badge variant={sample.state === 'ok' ? 'secondary' : 'warning'}>
                        {t(`state.${sample.state}`)}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {totalPages > 1 && (
          <div className="mt-3 flex items-center justify-end gap-2">
            <Button variant="ghost" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
              {t('usage.previous')}
            </Button>
            <TypographyText tone="muted" size="xs" as="span">
              {t('usage.pageOf', { page, totalPages })}
            </TypographyText>
            <Button
              variant="ghost"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            >
              {t('usage.next')}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
