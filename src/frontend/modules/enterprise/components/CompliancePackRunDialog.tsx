// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { useStartCompliancePackRun } from '../hooks/useCompliancePacks';

type CompliancePackRunDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  framework: string;
  frameworkName: string;
  environmentId?: string;
};

export function CompliancePackRunDialog({
  open: isOpen,
  onOpenChange,
  framework,
  frameworkName,
  environmentId,
}: CompliancePackRunDialogProps) {
  const { t } = useTranslation('compliancepacks');
  const mutation = useStartCompliancePackRun();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('runDialog.title')}
      description={t('runDialog.body')}
      confirmLabel={t('runDialog.confirm')}
      variant="default"
      permission={PERM.COMPLIANCE_MANAGE}
      mutation={mutation}
      variables={{ framework, environmentId }}
    >
      <div data-testid="compliance-pack-run-target" className="rounded-md border border-border bg-muted/40 p-3 text-xs">
        <TypographyText size="sm">{frameworkName}</TypographyText>
        <TypographyText tone="muted" size="sm">
          {t('runDialog.environment')}: {environmentId || t('run.defaultEnvironment')}
        </TypographyText>
      </div>
    </MutationConfirmDialog>
  );
}
