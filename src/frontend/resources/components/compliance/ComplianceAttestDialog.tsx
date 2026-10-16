// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { Label } from '@resources/components/ui/Label';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Textarea } from '@resources/components/ui/Textarea';
import { Text as UiText } from '@resources/components/ui/Typography';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { useCreateAttestation } from '@resources/hooks/useComplianceAttestations';

export type AttestedRun = {
  id: string;
  startedAt: string;
};

type ComplianceAttestDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  framework: string;
  frameworkName: string;
  run?: AttestedRun;
};

export function ComplianceAttestDialog({
  open: isOpen,
  onOpenChange,
  framework,
  frameworkName,
  run,
}: ComplianceAttestDialogProps) {
  const { t } = useTranslation('compliancepacks');
  const prefs = useLocaleFormat();
  const mutation = useCreateAttestation();
  const [note, setNote] = useState('');
  const noteId = useId();
  const hintId = useId();

  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (isOpen) setNote('');
  }

  const trimmed = note.trim();

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={t('attestation.dialog.title')}
      description={t('attestation.dialog.body')}
      confirmLabel={t('attestation.dialog.confirm')}
      variant="default"
      permission={PERM.COMPLIANCE_ATTEST}
      mutation={mutation}
      variables={{ framework, runId: run?.id ?? '', note: trimmed }}
      disabled={trimmed === ''}
    >
      <div data-testid="compliance-attest-target" className="space-y-1 rounded-md border border-border bg-muted/40 p-3">
        <UiText size="sm">
          {t('attestation.dialog.framework')}: {frameworkName}
        </UiText>
        <UiText tone="muted" size="sm">
          {t('attestation.dialog.run')}:{' '}
          {run ? t('attestation.run', { id: run.id, time: formatDate(run.startedAt, prefs) }) : t('attestation.noRun')}
        </UiText>
      </div>
      <div className="space-y-1">
        <Label htmlFor={noteId}>{t('attestation.dialog.note')}</Label>
        <Textarea
          id={noteId}
          value={note}
          required
          aria-describedby={hintId}
          placeholder={t('attestation.dialog.notePlaceholder')}
          onChange={(evt) => setNote(evt.target.value)}
        />
        <UiText id={hintId} tone="muted" size="sm">
          {t('attestation.dialog.noteRequired')}
        </UiText>
      </div>
    </MutationConfirmDialog>
  );
}
