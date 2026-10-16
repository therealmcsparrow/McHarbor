// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { usePluginFieldOptions, type KindField, type PluginKind } from '../hooks/usePluginKinds';

type Props = {
  kind: PluginKind;
  field: KindField;
  env: string;
  value: string;
  onChange: (value: string) => void;
};

export function PluginSpecField({ kind, field, env, value, onChange }: Props) {
  const { t } = useTranslation('reconciler');
  const dynamic = usePluginFieldOptions(kind, field, env);
  const id = `spec-${field.key}`;
  const label = field.unit ? `${field.label} (${field.unit})` : field.label;
  const notManaged = { value: '', label: t('plugin.notManaged') };

  let control;
  if (field.type === 'bool') {
    control = (
      <Select
        value={value}
        onChange={onChange}
        ariaLabel={label}
        options={[notManaged, { value: 'true', label: t('plugin.yes') }, { value: 'false', label: t('plugin.no') }]}
      />
    );
  } else if (field.type === 'select') {
    const opts = field.dynamic ? (dynamic.data ?? []) : (field.options ?? []);
    const withCurrent = value && !opts.some((o) => o.value === value) ? [{ value, label: value }, ...opts] : opts;
    control = (
      <Select
        value={value}
        onChange={onChange}
        ariaLabel={label}
        searchable={withCurrent.length > 8}
        options={[
          notManaged,
          ...withCurrent.map((o) => ({ value: o.value, label: o.label || o.value, description: o.detail })),
        ]}
      />
    );
  } else if (field.type === 'number') {
    control = (
      <NumberInput
        id={id}
        nullable
        value={toNumberOrNull(value)}
        min={field.min}
        max={field.max ? field.max : undefined}
        placeholder={t('plugin.notManaged')}
        onChange={(next) => onChange(toNumberText(next))}
      />
    );
  } else {
    control = (
      <Input
        id={id}
        type="text"
        value={value}
        placeholder={t('plugin.notManaged')}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }

  return (
    <div>
      <Label htmlFor={id} className="mb-1">
        {label}
      </Label>
      {control}
      {field.description && <p className="mt-1 text-xs text-muted-foreground">{field.description}</p>}
    </div>
  );
}
