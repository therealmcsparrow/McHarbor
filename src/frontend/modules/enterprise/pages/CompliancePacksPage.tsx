// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlayerPlay } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Select } from '@resources/components/ui/Select';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { StatusError } from '@resources/components/ui/StatusError';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { PageHeader } from '@resources/layout/PageHeader';
import { formatDate } from '@resources/utils/format';
import { ComplianceAttestationPanel } from '@resources/components/compliance/ComplianceAttestationPanel';
import { CompliancePackControls } from '../components/CompliancePackControls';
import { CompliancePackRunDialog } from '../components/CompliancePackRunDialog';
import type { ComplianceStatus } from '../hooks/useCompliance';
import { useCompliancePackRun, useCompliancePacks } from '../hooks/useCompliancePacks';
import { STATUS_VARIANT } from '../components/ComplianceSectionCard';

const SUMMARY_ORDER: ComplianceStatus[] = ['fail', 'warn', 'unknown', 'pass', 'info'];

export default function CompliancePacksPage() {
  const { t } = useTranslation('compliancepacks');
  const prefs = useLocaleFormat();
  const canView = usePermission(PERM.COMPLIANCE_VIEW);
  const packs = useCompliancePacks(canView);
  const [selected, setSelected] = useState('');
  const [runOpen, setRunOpen] = useState(false);

  const list = packs.data ?? [];
  const pack = list.find((entry) => entry.framework.key === selected) ?? list[0];
  const run = useCompliancePackRun(pack?.latestRun?.id);

  if (!canView) {
    return (
      <div className="space-y-6">
        <PageHeader title={t('title')} description={t('description')} headingLeaf />
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.COMPLIANCE_VIEW })}</Alert>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} headingLeaf />
      <Alert variant="info">{t('disclaimer')}</Alert>
      {packs.isLoading ? (
        <Stack gap={4}>
          <Skeleton className="h-16" />
          <Skeleton className="h-64" />
        </Stack>
      ) : packs.isError ? (
        <ErrorState title={t('title')} error={packs.error} onRetry={() => packs.refetch()} />
      ) : pack ? (
        <>
          <Flex align="center" justify="between" gap={3} wrap>
            <Flex align="center" gap={3} wrap>
              <Select
                ariaLabel={t('framework.label')}
                value={pack.framework.key}
                onChange={setSelected}
                searchable={false}
                options={list.map((entry) => ({ value: entry.framework.key, label: entry.framework.name }))}
              />
              {pack.latestRun ? (
                <>
                  {SUMMARY_ORDER.filter((currentStatus) => pack.latestRun!.summary[currentStatus] > 0).map(
                    (statusValue) => (
                      <Badge key={statusValue} variant={STATUS_VARIANT[statusValue]}>
                        {t(`summary.${statusValue}`)}: {pack.latestRun!.summary[statusValue]}
                      </Badge>
                    ),
                  )}
                  <TypographyText tone="muted" size="sm">
                    {t('run.last', { time: formatDate(pack.latestRun.startedAt, prefs) })}
                  </TypographyText>
                  <Badge variant="secondary">{t(`run.status.${pack.latestRun.status}`)}</Badge>
                  <Badge variant="secondary">{t(`run.trigger.${pack.latestRun.trigger}`)}</Badge>
                  <StatusError error={pack.latestRun.lastError} />
                </>
              ) : (
                <TypographyText tone="muted" size="sm">
                  {t('run.never')}
                </TypographyText>
              )}
            </Flex>
            {pack.framework.runnable ? (
              <Button permission={PERM.COMPLIANCE_MANAGE} onClick={() => setRunOpen(true)}>
                <IconPlayerPlay className="size-4" aria-hidden="true" />
                {t('actions.runChecks')}
              </Button>
            ) : (
              <TypographyText tone="muted" size="sm">
                {t('framework.mappingOnly')}
              </TypographyText>
            )}
          </Flex>
          {pack.latestRun && pack.latestRun.summary.unknown > 0 ? (
            <Alert variant="warning">
              {t('notAssessed.title')} — {t('notAssessed.body', { count: pack.latestRun.summary.unknown })}
            </Alert>
          ) : null}
          <ComplianceAttestationPanel
            framework={pack.framework.key}
            frameworkName={pack.framework.name}
            run={pack.latestRun}
          />
          {!pack.latestRun ? <Alert variant="info">{t('table.empty')}</Alert> : null}
          <CompliancePackControls controls={pack.framework.controls} results={run.data?.results ?? []} />
          <CompliancePackRunDialog
            open={runOpen}
            onOpenChange={setRunOpen}
            framework={pack.framework.key}
            frameworkName={pack.framework.name}
          />
        </>
      ) : null}
    </div>
  );
}
