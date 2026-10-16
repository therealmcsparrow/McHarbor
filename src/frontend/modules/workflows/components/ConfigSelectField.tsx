// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { ConfigFieldLabelled } from './ConfigFieldLabelled';
import { ResourcePicker } from './ResourcePicker';
import type { ConfigField } from '../types';

interface ConfigSelectFieldProps {
  field: ConfigField;
  strVal: string;
  onChange: (v: unknown) => void;
  fieldLabel: string;
  optionLabel: (optValue: string, fallback: string) => string;
}

export function ConfigSelectField({ field, strVal, onChange, fieldLabel, optionLabel }: ConfigSelectFieldProps) {
  const { t } = useTranslation('common');

  if (field.allowCustom) {
    // Presets plus a typed value: the resource picker already owns
    // that interaction, so a select with allowCustom borrows it.
    return (
      <ConfigFieldLabelled label={fieldLabel} required={field.required}>
        <ResourcePicker
          value={strVal}
          onChange={(v) => onChange(v)}
          allowCustom
          placeholder={t('workflows.selectPlaceholder')}
          emptyLabel={t('select.noResults')}
          items={(field.options ?? []).map((opt) => ({
            value: opt.value,
            title: optionLabel(opt.value, opt.label),
            subtitle: opt.value !== opt.label ? opt.value : undefined,
          }))}
        />
      </ConfigFieldLabelled>
    );
  }

  return (
    <ConfigFieldLabelled label={fieldLabel} required={field.required}>
      <Select
        value={strVal}
        onChange={onChange}
        options={(field.options ?? []).map((opt) => ({
          value: opt.value,
          label: optionLabel(opt.value, opt.label),
        }))}
        placeholder={t('workflows.selectPlaceholder')}
        ariaLabel={field.label}
        variant="outline"
        className="text-xs"
      />
    </ConfigFieldLabelled>
  );
}
