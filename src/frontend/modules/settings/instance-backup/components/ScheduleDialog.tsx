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
import { NumberInput } from '@resources/components/ui/NumberInput';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Select } from '@resources/components/ui/Select';
import { MultiSelect } from '@resources/components/ui/MultiSelect';
import { Switch } from '@resources/components/ui/Switch';
import { Alert } from '@resources/components/ui/Alert';
import { FileUpload } from '@resources/components/ui/FileUpload';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { useStorageLocations } from '@resources/hooks/useStorageLocations';
import { PERM } from '@core/auth/permissionKeys';
import { InstanceBackupRefusal, type KeyKind } from '../hooks/useInstanceBackups';
import { useSaveSchedule, type ExportSchedule, type ScheduleInput } from '../hooks/useExportSchedules';

type ScheduleDialogProps = {
  open: boolean;
  schedule: ExportSchedule | null;
  onOpenChange: (open: boolean) => void;
};

function initial(s: ExportSchedule | null): ScheduleInput {
  return s
    ? {
        name: s.name,
        cron: s.cron,
        enabled: s.enabled,
        includeMetrics: s.includeMetrics,
        locationIds: s.locationIds ?? [],
        retentionCount: s.retentionCount,
        retentionDays: s.retentionDays,
        verifyAfter: s.verifyAfter,
      }
    : {
        name: '',
        cron: '0 3 * * *',
        enabled: true,
        includeMetrics: false,
        locationIds: [],
        retentionCount: 7,
        retentionDays: 0,
        verifyAfter: true,
      };
}

const SWITCHES = [
  { id: 'ib-s-verify', key: 'verifyAfter' as const, label: 'schedules.verifyAfter' },
  { id: 'ib-s-metrics', key: 'includeMetrics' as const, label: 'create.metrics' },
  { id: 'ib-s-enabled', key: 'enabled' as const, label: 'schedules.enabled' },
];

export function ScheduleDialog({ open: isOpen, schedule, onOpenChange }: ScheduleDialogProps) {
  const { t } = useTranslation('instancebackup');
  const [form, setForm] = useState<ScheduleInput>(() => initial(schedule));
  const [passphrase, setPassphrase] = useState('');
  const [keyKind, setKeyKind] = useState<KeyKind>(schedule?.keyKind ?? 'passphrase');
  const [keyFile, setKeyFile] = useState('');
  const save = useSaveSchedule();
  const locations = useStorageLocations();
  const set = (patch: Partial<ScheduleInput>) => setForm((f) => ({ ...f, ...patch }));
  const creating = schedule === null;
  const given = keyKind === 'keyfile' ? keyFile !== '' : passphrase !== '';
  const valid = keyKind === 'keyfile' ? keyFile !== '' : Array.from(passphrase).length >= 12;
  const keyOK = creating ? valid : !given || valid;
  const key = !given ? {} : keyKind === 'keyfile' ? { keyFile } : { passphrase };

  const submit = () =>
    save.mutate({ id: schedule?.id, input: { ...form, ...key } }, { onSuccess: () => onOpenChange(false) });
  const message =
    save.error instanceof InstanceBackupRefusal ? t(`schedules.reasons.${save.error.reason}`) : save.error?.message;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t(creating ? 'schedules.createTitle' : 'schedules.editTitle')}</DialogTitle>
          <DialogDescription>{t('schedules.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="ib-s-name">{t('schedules.name')}</Label>
            <Input id="ib-s-name" maxLength={100} value={form.name} onChange={(e) => set({ name: e.target.value })} />
          </div>
          <ReadableScheduleField value={form.cron} label={t('schedules.when')} onChange={(cron) => set({ cron })} />
          <div className="flex flex-col gap-1.5">
            <Label>{t(creating ? 'schedules.key' : 'schedules.replaceKey')}</Label>
            <Select
              value={keyKind}
              onChange={(value) => setKeyKind(value as KeyKind)}
              ariaLabel={t('schedules.key')}
              options={[
                { value: 'passphrase', label: t('keyKind.passphrase') },
                { value: 'keyfile', label: t('keyKind.keyfile') },
              ]}
            />
            {keyKind === 'passphrase' ? (
              <PasswordInput
                aria-label={t('create.passphrase')}
                autoComplete="new-password"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
              />
            ) : (
              <FileUpload
                accept=".key,.txt"
                label={t('restore.keyFile')}
                onChange={(files) => {
                  const f = files[0];
                  if (!f) return setKeyFile('');
                  void f.text().then(setKeyFile);
                }}
              />
            )}
            <p className="text-xs text-muted-foreground">{t('schedules.keyHint')}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label>{t('schedules.location')}</Label>
            <MultiSelect
              value={form.locationIds ?? []}
              onChange={(next) => set({ locationIds: next })}
              ariaLabel={t('schedules.location')}
              emptyLabel={t('schedules.hostOnly')}
              options={(locations.data ?? []).filter((l) => l.enabled).map((l) => ({ value: l.id, label: l.name }))}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="ib-s-count">{t('schedules.keepCount')}</Label>
              <NumberInput
                id="ib-s-count"
                min={0}
                max={1000}
                value={form.retentionCount}
                onChange={(v) => set({ retentionCount: v })}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="ib-s-days">{t('schedules.keepDays')}</Label>
              <NumberInput
                id="ib-s-days"
                min={0}
                max={3650}
                value={form.retentionDays}
                onChange={(v) => set({ retentionDays: v })}
              />
            </div>
          </div>
          {SWITCHES.map((sw) => (
            <div key={sw.id} className="flex items-center justify-between gap-4">
              <Label htmlFor={sw.id}>{t(sw.label)}</Label>
              <Switch id={sw.id} checked={form[sw.key]} onCheckedChange={(v) => set({ [sw.key]: v })} />
            </div>
          ))}
          {message && <Alert variant="destructive">{message}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.INSTANCE_BACKUP_MANAGE}
            disabled={!keyOK || form.name.trim() === '' || save.isPending}
            onClick={submit}
          >
            {t('actions.save', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
