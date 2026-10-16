// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useRecordStep } from '@resources/hooks/useRunbooks';
import type { RunbookStep } from '@resources/types/runbooks';

type RunbookActionDialogProps = {
  runId: string;
  step: RunbookStep;
  finding: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function RunbookActionDialog({
  runId,
  step,
  finding,
  open: isOpen,
  onOpenChange,
  onSuccess,
}: RunbookActionDialogProps) {
  const { t } = useTranslation('runbooks');
  const record = useRecordStep(runId);

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('run.confirmActionTitle', { title: step.title })}
      description={t('run.confirmActionDescription', { action: step.actionKey ?? '' })}
      confirmLabel={t('run.runStep')}
      permission={step.requiredPermissions?.length ? step.requiredPermissions : undefined}
      mutation={record}
      variables={{ stepId: step.id, state: 'done' as const, finding }}
      onSuccess={onSuccess}
    />
  );
}
