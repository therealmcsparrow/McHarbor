// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDeviceFloppy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import { SecretFileModeToggle } from '@resources/components/secrets/SecretFileModeToggle';
import { SecretPicker } from '@resources/components/secrets/SecretPicker';
import { PERM } from '@core/auth/permissionKeys';

type EnvEntry = { key: string; value: string };

type EnvironmentTabEditorProps = {
  entries: EnvEntry[];
  onEntriesChange: (entries: EnvEntry[]) => void;
  onSave: () => void;
  saving: boolean;
  secretFileMode: boolean;
  secretFileModeDisabled: boolean;
  onSecretFileModeChange: (secretFileMode: boolean) => void;
};

export function EnvironmentTabEditor({
  entries,
  onEntriesChange,
  onSave,
  saving,
  secretFileMode,
  secretFileModeDisabled,
  onSecretFileModeChange,
}: EnvironmentTabEditorProps) {
  const { t } = useTranslation('stacks');
  const { t: ts } = useTranslation('secrets');
  const pickerLabel = ts('picker.label');

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">{t('detail.envVars')}</h3>
        <Button permission={PERM.STACKS_MANAGE} size="sm" onClick={onSave} disabled={saving}>
          <IconDeviceFloppy className="mr-1 size-3.5" />
          {saving ? t('editStack.saving') : t('editStack.saveStack')}
        </Button>
      </div>
      <KeyValueEditor
        entries={entries}
        onChange={onEntriesChange}
        keyLabel={t('detail.key')}
        valueLabel={t('detail.value')}
        addLabel={t('detail.key')}
      />
      {/*
        Saved on its own rather than with the variables: it changes how the
        next deploy delivers them, so the stack has to carry it whether or
        not the operator also edited a value.
      */}
      <div className="border-t border-border pt-4">
        <SecretFileModeToggle
          checked={secretFileMode}
          disabled={secretFileModeDisabled}
          onCheckedChange={onSecretFileModeChange}
        />
      </div>

      {/*
        The picker writes a ${secret:NAME} reference into the last row rather
        than a value: it cannot read values, and the substitution happens
        server-side at deploy time.
      */}
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">{pickerLabel}</span>
        <SecretPicker
          className="w-72"
          value=""
          onChange={(reference) => onEntriesChange([...entries, { key: '', value: reference }])}
        />
      </div>
    </div>
  );
}
