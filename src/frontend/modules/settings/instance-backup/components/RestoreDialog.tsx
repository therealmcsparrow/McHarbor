// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { InstanceBackupRefusal, type InstanceExport } from '../hooks/useInstanceBackups';
import {
  usePlanRestore,
  useStartRestore,
  useUploadExport,
  type RestoreInput,
  type RestorePlan,
  type RestoreSource,
} from '../hooks/useInstanceRestore';
import { canPlan, RestoreSourceStep, type RestoreForm } from './RestoreSourceStep';
import { CONFIRM_WORD, RestorePlanStep } from './RestorePlanStep';

type RestoreDialogProps = {
  open: boolean;
  initialExportId: string;
  exports: InstanceExport[];
  onOpenChange: (open: boolean) => void;
};

const EMPTY: RestoreForm = {
  exportId: '',
  file: null,
  keyKind: 'passphrase',
  passphrase: '',
  keyFile: '',
  mode: 'full',
  categories: [],
  includeAudit: false,
  conflict: 'replace',
};

export function RestoreDialog({ open: isOpen, initialExportId, exports, onOpenChange }: RestoreDialogProps) {
  const { t } = useTranslation('instancebackup');
  const client = useQueryClient();
  const [form, setForm] = useState<RestoreForm>({ ...EMPTY, exportId: initialExportId });
  const [input, setInput] = useState<RestoreInput | null>(null);
  const [plan, setPlan] = useState<RestorePlan | null>(null);
  const [confirmation, setConfirmation] = useState('');
  const upload = useUploadExport();
  const planRestore = usePlanRestore();
  const start = useStartRestore();

  const closeDialog = (next: boolean) => {
    if (!next) {
      setForm({ ...EMPTY, exportId: initialExportId });
      setInput(null);
      setPlan(null);
      setConfirmation('');
      upload.reset();
      planRestore.reset();
      start.reset();
    }
    onOpenChange(next);
  };

  const check = async () => {
    let source: RestoreSource | undefined = form.exportId ? { exportId: form.exportId } : undefined;
    if (!source && form.file) {
      const up = await upload.mutateAsync(form.file);
      source = { uploadId: up.uploadId };
    }
    if (!source) return;
    const next: RestoreInput = {
      source,
      ...(form.keyKind === 'passphrase' ? { passphrase: form.passphrase } : { keyFile: form.keyFile }),
      mode: form.mode,
      includeAudit: form.includeAudit,
      ...(form.mode === 'selective' ? { categories: form.categories, conflict: form.conflict } : {}),
    };
    setInput(next);
    setPlan(await planRestore.mutateAsync(next));
  };

  const restore = () => {
    if (!input) return;
    start.mutate(input, {
      onSuccess: () => {
        void client.invalidateQueries({ queryKey: ['instance-backups', 'restore-status'] });
        closeDialog(false);
      },
    });
  };

  const error = start.error ?? planRestore.error ?? upload.error;
  const message =
    error instanceof InstanceBackupRefusal ? error.detail || t(`restore.reasons.${error.reason}`) : error?.message;
  const busy = upload.isPending || planRestore.isPending || start.isPending;
  const ready = plan !== null && plan.refusals.length === 0 && confirmation === CONFIRM_WORD;

  return (
    <Dialog open={isOpen} onOpenChange={closeDialog}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('restore.title')}</DialogTitle>
          <DialogDescription>{plan ? t('restore.planDescription') : t('restore.description')}</DialogDescription>
        </DialogHeader>
        {plan ? (
          <RestorePlanStep plan={plan} confirmation={confirmation} onConfirmationChange={setConfirmation} />
        ) : (
          <RestoreSourceStep form={form} exports={exports} onChange={(patch) => setForm((f) => ({ ...f, ...patch }))} />
        )}
        {message && <Alert variant="destructive">{message}</Alert>}
        <DialogFooter>
          <Button variant="outline" onClick={() => (plan ? setPlan(null) : closeDialog(false))}>
            {plan ? t('actions.back', { ns: 'common' }) : t('actions.cancel', { ns: 'common' })}
          </Button>
          {plan ? (
            <Button
              variant="destructive"
              permission={PERM.INSTANCE_BACKUP_RESTORE}
              disabled={!ready || busy}
              onClick={restore}
            >
              {t('restore.submit')}
            </Button>
          ) : (
            <Button
              permission={PERM.INSTANCE_BACKUP_RESTORE}
              disabled={!canPlan(form) || busy}
              onClick={() => void check().catch(() => undefined)}
            >
              {t('restore.check')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
