// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Label } from '@resources/components/ui/Label';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Textarea } from '@resources/components/ui/Textarea';
import { useDecideElevation, type ElevationRequest } from '@resources/hooks/useElevations';

type DecisionDialogProps = {
  request: ElevationRequest;
  requester: string;
  approve: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function DecisionDialog({ request, requester, approve, open: isOpen, onOpenChange }: DecisionDialogProps) {
  const { t } = useTranslation('oncall');
  const queryClient = useQueryClient();
  const decide = useDecideElevation();
  const [note, setNote] = useState('');
  const noteId = `decision-note-${request.id}`;

  const [wasOpen, setWasOpen] = useState(false);
  if (isOpen !== wasOpen) {
    setWasOpen(isOpen);
    if (isOpen) setNote('');
  }

  const values = {
    requester,
    permission: request.permissionLabel || request.permissionKey,
    minutes: request.requestedMinutes,
  };
  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={approve ? t('approvals.approveTitle') : t('approvals.denyTitle')}
      description={approve ? t('approvals.approveDescription', values) : t('approvals.denyDescription', values)}
      confirmLabel={approve ? t('approvals.approve') : t('approvals.deny')}
      variant={approve ? 'default' : 'destructive'}
      permission={PERM.USERS_MANAGE}
      mutation={decide}
      variables={{ id: request.id, approve, note }}
      onSuccess={() => void queryClient.invalidateQueries({ queryKey: ['oncall', 'elevations'] })}
    >
      <div className="flex flex-col gap-1">
        <Label htmlFor={noteId}>{t('approvals.note')}</Label>
        <Textarea id={noteId} value={note} onChange={(evt) => setNote(evt.target.value)} rows={2} />
      </div>
    </MutationConfirmDialog>
  );
}
