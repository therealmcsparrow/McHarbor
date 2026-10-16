// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconRefresh } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Flex, Grid } from '@resources/components/ui/Layout';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { PageHeader } from '@resources/layout/PageHeader';
import { formatDate } from '@resources/utils/format';
import { ComplianceReportDialog } from '../components/ComplianceReportDialog';
import { ComplianceSectionCard, STATUS_VARIANT } from '../components/ComplianceSectionCard';
import { type ComplianceStatus, useCompliancePosture } from '../hooks/useCompliance';

const SUMMARY_ORDER: ComplianceStatus[] = ['fail', 'warn', 'pass', 'info', 'unknown'];

export default function CompliancePage() {
  const { t } = useTranslation('compliance');
  const prefs = useLocaleFormat();
  const canView = usePermission(PERM.COMPLIANCE_VIEW);
  const posture = useCompliancePosture(canView);
  const [reportOpen, setReportOpen] = useState(false);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('title')}
        description={t('description')}
        headingLeaf
        actions={
          <>
            <Button
              variant="outline"
              permission={PERM.COMPLIANCE_VIEW}
              onClick={() => posture.refetch()}
              disabled={posture.isFetching}
            >
              <IconRefresh className="size-4" aria-hidden="true" />
              {t('refresh')}
            </Button>
            <Button permission={PERM.COMPLIANCE_VIEW} onClick={() => setReportOpen(true)} disabled={!posture.data}>
              <IconDownload className="size-4" aria-hidden="true" />
              {t('downloadReport')}
            </Button>
          </>
        }
      />
      {!canView ? (
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.COMPLIANCE_VIEW })}</Alert>
      ) : posture.isLoading ? (
        <Grid columns="grid-cols-1 lg:grid-cols-2" gap={4}>
          {SUMMARY_ORDER.map((s) => (
            <Skeleton key={s} className="h-40" />
          ))}
        </Grid>
      ) : posture.isError ? (
        <ErrorState title={t('title')} error={posture.error} onRetry={() => posture.refetch()} />
      ) : posture.data ? (
        <>
          <Flex align="center" gap={2} wrap>
            {SUMMARY_ORDER.filter((s) => posture.data.summary[s] > 0).map((s) => (
              <Badge key={s} variant={STATUS_VARIANT[s]}>
                {t(`summary.${s}`)}: {posture.data.summary[s]}
              </Badge>
            ))}
            <TypographyText tone="muted" size="sm">
              {t('generatedAt', { time: formatDate(posture.data.generatedAt, prefs) })}
            </TypographyText>
          </Flex>
          <Grid columns="grid-cols-1 lg:grid-cols-2" gap={4}>
            {posture.data.sections.map((section) => (
              <ComplianceSectionCard key={section.id} section={section} />
            ))}
          </Grid>
          <ComplianceReportDialog open={reportOpen} onOpenChange={setReportOpen} sections={posture.data.sections} />
        </>
      ) : null}
    </div>
  );
}
