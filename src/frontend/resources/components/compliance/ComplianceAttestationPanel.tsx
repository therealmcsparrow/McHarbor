// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSignature } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Text as UiText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { useLatestAttestation } from '@resources/hooks/useComplianceAttestations';
import { ComplianceAttestDialog, type AttestedRun } from './ComplianceAttestDialog';

type ComplianceAttestationPanelProps = {
  framework: string;
  frameworkName: string;
  run?: AttestedRun & { status: string };
  title?: string;
  description?: string;
};

export function ComplianceAttestationPanel({
  framework,
  frameworkName,
  run,
  title,
  description,
}: ComplianceAttestationPanelProps) {
  const { t } = useTranslation('compliancepacks');
  const prefs = useLocaleFormat();
  const canView = usePermission(PERM.COMPLIANCE_VIEW);
  const latest = useLatestAttestation(framework, canView);
  const [isOpen, setOpen] = useState(false);
  const runOpen = run?.status === 'running';

  return (
    <section data-testid="compliance-attestation" className="space-y-3 rounded-lg border border-border bg-card p-4">
      <Flex align="start" justify="between" gap={3} wrap>
        <Stack gap={1}>
          <UiText weight="semibold">{title ?? t('attestation.title')}</UiText>
          <UiText tone="muted" size="sm">
            {description ?? t('attestation.description')}
          </UiText>
        </Stack>
        <Button permission={PERM.COMPLIANCE_ATTEST} disabled={runOpen} onClick={() => setOpen(true)}>
          <IconSignature className="size-4" aria-hidden="true" />
          {t('attestation.action')}
        </Button>
      </Flex>
      {runOpen ? <Alert variant="info">{t('attestation.runOpen')}</Alert> : null}
      {!canView ? (
        <UiText tone="muted" size="sm">
          {t('permission.requires', { ns: 'common', permission: PERM.COMPLIANCE_VIEW })}
        </UiText>
      ) : latest.isLoading ? (
        <Skeleton className="h-10" />
      ) : latest.isError ? (
        <Alert variant="destructive">{latest.error.message}</Alert>
      ) : latest.data ? (
        <Stack gap={1}>
          <UiText size="sm">
            {t('attestation.last', {
              time: formatDate(latest.data.attestedAt, prefs),
              user: latest.data.attestedByName || latest.data.attestedBy,
            })}
          </UiText>
          <UiText tone="muted" size="sm">
            {latest.data.runId ? (
              <>
                {t('attestation.dialog.run')}: {latest.data.runId}
              </>
            ) : (
              t('attestation.noRun')
            )}
          </UiText>
          <UiText size="sm" className="break-words whitespace-pre-wrap">
            {latest.data.note}
          </UiText>
        </Stack>
      ) : (
        <UiText tone="muted" size="sm">
          {t('attestation.never')}
        </UiText>
      )}
      <ComplianceAttestDialog
        open={isOpen}
        onOpenChange={setOpen}
        framework={framework}
        frameworkName={frameworkName}
        run={run && !runOpen ? { id: run.id, startedAt: run.startedAt } : undefined}
      />
    </section>
  );
}
