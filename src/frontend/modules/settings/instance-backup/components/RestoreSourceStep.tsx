// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { FileUpload } from '@resources/components/ui/FileUpload';
import { Alert } from '@resources/components/ui/Alert';
import type { InstanceExport, KeyKind } from '../hooks/useInstanceBackups';
import { RESTORE_CATEGORIES, type Conflict, type RestoreMode } from '../hooks/useInstanceRestore';

export type RestoreForm = {
  exportId: string;
  file: File | null;
  keyKind: KeyKind;
  passphrase: string;
  keyFile: string;
  mode: RestoreMode;
  categories: string[];
  includeAudit: boolean;
  conflict: Conflict;
};

export function canPlan(form: RestoreForm): boolean {
  const source = form.exportId !== '' || form.file !== null;
  const key = form.keyKind === 'passphrase' ? form.passphrase !== '' : form.keyFile !== '';
  const scope = form.mode !== 'selective' || form.categories.length > 0 || form.includeAudit;
  return source && key && scope;
}

type RestoreSourceStepProps = {
  form: RestoreForm;
  exports: InstanceExport[];
  onChange: (patch: Partial<RestoreForm>) => void;
};

export function RestoreSourceStep({ form, exports, onChange }: RestoreSourceStepProps) {
  const { t } = useTranslation('instancebackup');
  const toggle = (category: string) =>
    onChange({
      categories: form.categories.includes(category)
        ? form.categories.filter((c) => c !== category)
        : [...form.categories, category],
    });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <Label>{t('restore.source')}</Label>
        <Select
          value={form.exportId}
          onChange={(value) => onChange({ exportId: value, file: null })}
          ariaLabel={t('restore.source')}
          options={[
            { value: '', label: t('restore.uploadOption') },
            ...exports
              .filter((e) => e.status === 'completed')
              .map((e) => ({ value: e.id, label: e.note ? `${e.createdAt} · ${e.note}` : e.createdAt })),
          ]}
        />
        {form.exportId === '' && (
          <FileUpload
            accept=".mchx"
            label={t('restore.file')}
            onChange={(files) => onChange({ file: files[0] ?? null })}
          />
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label>{t('restore.keyKind')}</Label>
        <Select
          value={form.keyKind}
          onChange={(value) => onChange({ keyKind: value as KeyKind })}
          ariaLabel={t('restore.keyKind')}
          options={[
            { value: 'passphrase', label: t('keyKind.passphrase') },
            { value: 'keyfile', label: t('keyKind.keyfile') },
          ]}
        />
        {form.keyKind === 'passphrase' ? (
          <PasswordInput
            aria-label={t('create.passphrase')}
            autoComplete="off"
            value={form.passphrase}
            onChange={(e) => onChange({ passphrase: e.target.value })}
          />
        ) : (
          <FileUpload
            accept=".key,.txt"
            label={t('restore.keyFile')}
            onChange={(files) => {
              const f = files[0];
              if (!f) return onChange({ keyFile: '' });
              void f.text().then((text) => onChange({ keyFile: text }));
            }}
          />
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label>{t('restore.mode')}</Label>
        <Select
          value={form.mode}
          onChange={(value) => onChange({ mode: value as RestoreMode })}
          ariaLabel={t('restore.mode')}
          options={(['full', 'migration', 'selective'] as const).map((m) => ({
            value: m,
            label: t(`restore.modes.${m}.label`),
            description: t(`restore.modes.${m}.description`),
          }))}
        />
      </div>
      {form.mode === 'selective' && (
        <div className="grid grid-cols-2 gap-2">
          {RESTORE_CATEGORIES.map((c) => (
            <div key={c} className="flex items-center gap-2">
              <Checkbox id={`ib-cat-${c}`} checked={form.categories.includes(c)} onCheckedChange={() => toggle(c)} />
              <Label htmlFor={`ib-cat-${c}`}>{t(`categories.${c}`)}</Label>
            </div>
          ))}
          <div className="col-span-2 flex flex-col gap-1.5">
            <Label>{t('restore.conflict')}</Label>
            <Select
              value={form.conflict}
              onChange={(value) => onChange({ conflict: value as Conflict })}
              ariaLabel={t('restore.conflict')}
              options={[
                { value: 'replace', label: t('restore.conflicts.replace') },
                { value: 'keep', label: t('restore.conflicts.keep') },
              ]}
            />
          </div>
        </div>
      )}
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-0.5">
          <Label htmlFor="ib-audit">{t('restore.includeAudit')}</Label>
          <p className="text-xs text-muted-foreground">{t('restore.includeAuditHelp')}</p>
        </div>
        <Switch id="ib-audit" checked={form.includeAudit} onCheckedChange={(v) => onChange({ includeAudit: v })} />
      </div>
      {form.includeAudit && <Alert variant="warning">{t('restore.auditApproval')}</Alert>}
    </div>
  );
}
