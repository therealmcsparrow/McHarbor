// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
import { Input } from '@resources/components/ui/Input';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Select } from '@resources/components/ui/Select';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import { Switch } from '@resources/components/ui/Switch';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Alert } from '@resources/components/ui/Alert';
import { PERM } from '@core/auth/permissionKeys';
import { useStorageLocations } from '@resources/hooks/useStorageLocations';
import { InstanceBackupRefusal, useCreateExport, useGenerateKeyFile, type KeyKind } from '../hooks/useInstanceBackups';

type CreateExportDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export const MIN_PASSPHRASE = 12;

export type ExportForm = {
  kind: KeyKind;
  passphrase: string;
  confirm: string;
  keyFile: string;
  keyFileSaved: boolean;
};

export function canSubmit(form: ExportForm): boolean {
  if (form.kind === 'passphrase') {
    return Array.from(form.passphrase).length >= MIN_PASSPHRASE && form.passphrase === form.confirm;
  }
  return form.keyFile !== '' && form.keyFileSaved;
}

const EMPTY: ExportForm = { kind: 'passphrase', passphrase: '', confirm: '', keyFile: '', keyFileSaved: false };

function saveText(itemName: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = itemName;
  link.click();
  URL.revokeObjectURL(url);
}

export function CreateExportDialog({ open: isOpen, onOpenChange }: CreateExportDialogProps) {
  const { t } = useTranslation('instancebackup');
  const [form, setForm] = useState<ExportForm>(EMPTY);
  const [keyFileName, setKeyFileName] = useState('');
  const [metrics, setMetrics] = useState(false);
  const [note, setNote] = useState('');
  // Empty keeps the export on this host, which is the default: an offsite copy
  // is an addition, and a failed upload must never be able to lose the backup.
  const [locationIds, setLocationIds] = useState<string[]>([]);
  const locations = useStorageLocations();
  const create = useCreateExport();
  const generate = useGenerateKeyFile();
  const set = (patch: Partial<ExportForm>) => setForm((current) => ({ ...current, ...patch }));

  const closeDialog = (next: boolean) => {
    if (!next) {
      setForm(EMPTY);
      setKeyFileName('');
      setMetrics(false);
      setNote('');
      setLocationIds([]);
      create.reset();
      generate.reset();
    }
    onOpenChange(next);
  };

  const submit = () => {
    if (!canSubmit(form)) return;
    create.mutate(
      {
        ...(form.kind === 'passphrase' ? { passphrase: form.passphrase } : { keyFile: form.keyFile }),
        includeMetrics: metrics,
        note: note.trim() || undefined,
        locationIds: locationIds.length > 0 ? locationIds : undefined,
      },
      { onSuccess: () => closeDialog(false) },
    );
  };

  const refusal =
    create.error instanceof InstanceBackupRefusal ? t(`reasons.${create.error.reason}`) : create.error?.message;
  const mismatch = form.confirm !== '' && form.passphrase !== form.confirm;

  return (
    <Dialog open={isOpen} onOpenChange={closeDialog}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label>{t('create.keyKind')}</Label>
            <Select
              value={form.kind}
              onChange={(value) => set({ kind: value as KeyKind })}
              options={[
                { value: 'passphrase', label: t('create.passphraseOption') },
                { value: 'keyfile', label: t('create.keyFileOption') },
              ]}
              ariaLabel={t('create.keyKind')}
            />
          </div>
          {form.kind === 'passphrase' ? (
            <>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="ib-passphrase">{t('create.passphrase')}</Label>
                <PasswordInput
                  id="ib-passphrase"
                  autoComplete="new-password"
                  value={form.passphrase}
                  onChange={(e) => set({ passphrase: e.target.value })}
                />
                <p className="text-xs text-muted-foreground">{t('create.passphraseHint')}</p>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="ib-confirm">{t('create.confirm')}</Label>
                <PasswordInput
                  id="ib-confirm"
                  autoComplete="new-password"
                  value={form.confirm}
                  onChange={(e) => set({ confirm: e.target.value })}
                />
                {mismatch && <p className="text-xs text-destructive">{t('create.mismatch')}</p>}
              </div>
            </>
          ) : (
            <div className="flex flex-col gap-3">
              {form.keyFile === '' ? (
                <Button
                  variant="outline"
                  disabled={generate.isPending}
                  onClick={() =>
                    generate.mutate(undefined, {
                      onSuccess: (res) => {
                        set({ keyFile: res.keyFile, keyFileSaved: false });
                        setKeyFileName(res.fileName);
                      },
                    })
                  }
                >
                  {t('create.generate')}
                </Button>
              ) : (
                <>
                  <Alert variant="warning">{t('create.keyFileReady')}</Alert>
                  <Button variant="outline" onClick={() => saveText(keyFileName, form.keyFile)}>
                    {t('create.saveKeyFile')}
                  </Button>
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id="ib-saved"
                      checked={form.keyFileSaved}
                      onCheckedChange={(v) => set({ keyFileSaved: v === true })}
                    />
                    <Label htmlFor="ib-saved">{t('create.keyFileSaved')}</Label>
                  </div>
                </>
              )}
            </div>
          )}
          <div className="flex items-start justify-between gap-4">
            <div className="flex flex-col gap-0.5">
              <Label htmlFor="ib-metrics">{t('create.metrics')}</Label>
              <p className="text-xs text-muted-foreground">{t('create.metricsHelp')}</p>
            </div>
            <Switch id="ib-metrics" checked={metrics} onCheckedChange={setMetrics} />
          </div>
          <div className="flex flex-col gap-1.5">
            {/* The label and the "host only" option are the schedule dialog's own
                strings: the choice is identical, so i18n-keys.md rule 2 says reuse
                rather than mint a second pair that would drift. */}
            <Label>{t('schedules.location')}</Label>
            <MultiSelect
              value={locationIds}
              onChange={setLocationIds}
              ariaLabel={t('schedules.location')}
              emptyLabel={t('schedules.hostOnly')}
              options={(locations.data ?? []).filter((l) => l.enabled).map((l) => ({ value: l.id, label: l.name }))}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ib-note">{t('create.note')}</Label>
            <Input
              id="ib-note"
              maxLength={500}
              value={note}
              placeholder={t('create.notePlaceholder')}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {refusal && <Alert variant="destructive">{refusal}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => closeDialog(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.INSTANCE_BACKUP_MANAGE}
            disabled={!canSubmit(form) || create.isPending}
            onClick={submit}
          >
            {t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
