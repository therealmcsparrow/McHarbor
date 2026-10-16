// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import { type DesiredState, type ReconcileMode, useEnforce, useSetMode } from '../hooks/useReconciler';

const MODES: ReconcileMode[] = ['observe', 'propose', 'enforce'];

type ModeDialogProps = {
  state: DesiredState;
  onOpenChange: (open: boolean) => void;
};

export function ModeDialog({ state, onOpenChange }: ModeDialogProps) {
  const { t } = useTranslation('reconciler');
  const [mode, setMode] = useState<ReconcileMode>(state.mode);
  const setPlain = useSetMode();
  const enforce = useEnforce();
  const pending = setPlain.isPending || enforce.isPending;
  const error = setPlain.error ?? enforce.error;
  const done = { onSuccess: () => onOpenChange(false) };

  const save = () => {
    if (pending || mode === state.mode) return;
    if (mode === 'enforce') enforce.mutate(state.id, done);
    else setPlain.mutate({ id: state.id, mode }, done);
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('mode.title', { name: state.name })}</DialogTitle>
          <DialogDescription>{t('mode.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('mode.label')}</Label>
            <Select
              value={mode}
              onChange={(value) => setMode(value as ReconcileMode)}
              ariaLabel={t('mode.label')}
              options={MODES.map((m) => ({ value: m, label: t(`mode.${m}`) }))}
            />
            <p className="mt-1 text-xs text-muted-foreground">{t(`mode.hint.${mode}`)}</p>
          </div>
          {mode === 'enforce' && state.mode !== 'enforce' && (
            <Alert variant="warning">{t('mode.enforceWarning')}</Alert>
          )}
          {error && <Alert variant="destructive">{error.message}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            variant={mode === 'enforce' ? 'destructive' : 'default'}
            permission={mode === 'enforce' ? PERM.RECONCILER_ENFORCE : PERM.RECONCILER_MANAGE}
            disabled={pending || mode === state.mode}
            onClick={save}
          >
            {mode === 'enforce' ? t('mode.confirmEnforce') : t('mode.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
