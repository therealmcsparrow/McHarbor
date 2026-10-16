// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { useDecideApproval, type ApprovalRequest } from '../../hooks/useApprovals';

type ApprovalDecisionDialogProps = {
  request: ApprovalRequest | null;
  approve: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ApprovalDecisionDialog({ request, approve, onOpenChange }: ApprovalDecisionDialogProps) {
  const { t } = useTranslation('security');
  const decide = useDecideApproval();
  const [note, setNote] = useState('');

  const values = {
    category: request ? t(`governance.categories.${request.category}`) : '',
    resource: request?.resourceName || request?.path || '',
    requester: request?.requesterName ?? '',
  };

  return (
    <MutationConfirmDialog
      open={request !== null}
      onOpenChange={(isOpen) => {
        if (!isOpen) setNote('');
        onOpenChange(isOpen);
      }}
      title={approve ? t('governance.approvals.approveTitle') : t('governance.approvals.denyTitle')}
      description={
        approve
          ? t('governance.approvals.approveDescription', values)
          : t('governance.approvals.denyDescription', values)
      }
      confirmLabel={approve ? t('governance.approvals.approve') : t('governance.approvals.deny')}
      variant={approve ? 'caution' : 'destructive'}
      permission={PERM.APPROVALS_DECIDE}
      mutation={decide}
      variables={{ id: request?.id ?? '', approve, note }}
    >
      <label className="block space-y-1">
        <span className="text-xs font-medium text-muted-foreground">{t('governance.approvals.note')}</span>
        <Textarea
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder={t('governance.approvals.notePlaceholder')}
          rows={3}
        />
      </label>
    </MutationConfirmDialog>
  );
}
