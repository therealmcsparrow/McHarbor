// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { Select } from '@resources/components/ui/Select';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import type { ConfigFieldDecl, ConfigFieldOption } from '../hooks/useCommunicationAdapters';

type SchemaFormFieldProps = {
  field: ConfigFieldDecl;
  value: string;
  onChange: (value: string) => void;
  /** i18next namespace of the plugin that owns the field, used for label_key / help_key. */
  i18nNs?: string;
};

/** Render a single ConfigFieldDecl as the appropriate input control.
 *
 * English label / help / placeholder come from the plugin's ConfigSchema
 * and are the fallback. When the field carries label_key / help_key and
 * i18nNs names the owning plugin's namespace (the plugin loader merges
 * every plugin bundle into i18next at boot), the key is resolved there. A key that does not resolve falls back to the
 * English text; placeholders are never translated. */
export function SchemaFormField({ field, value, onChange, i18nNs }: SchemaFormFieldProps) {
  const { t, i18n } = useTranslation('settings');
  const pluginText = (key: string | undefined, fallback: string | undefined) => {
    if (!key || !i18nNs || !i18n.exists(key, { ns: i18nNs })) return fallback;
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: the key is declared by the plugin and cannot be enumerated by the host
    return t(key, { ns: i18nNs });
  };
  const label = pluginText(field.label_key, field.label) ?? field.label;
  const placeholder = field.placeholder;
  const help = pluginText(field.help_key, field.help);

  switch (field.type) {
    case 'secret':
      return (
        <FieldShell label={label} help={help} required={field.required}>
          <Input
            type="password"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder ?? label}
          />
        </FieldShell>
      );

    case 'int':
      return (
        <FieldShell label={label} help={help} required={field.required}>
          <NumberInput
            nullable
            aria-label={label}
            value={toNumberOrNull(value)}
            min={field.min ?? undefined}
            max={field.max ?? undefined}
            onChange={(next) => onChange(toNumberText(next))}
            placeholder={placeholder ?? label}
          />
        </FieldShell>
      );

    case 'bool':
      return (
        <div className="flex items-center justify-between rounded-md border border-border bg-background p-3">
          <div>
            <p className="text-sm font-medium text-foreground">{label}</p>
            {help && <p className="mt-1 text-[11px] text-muted-foreground">{help}</p>}
          </div>
          <Switch
            checked={value === 'true' || value === '1'}
            onCheckedChange={(checked) => onChange(checked ? 'true' : 'false')}
          />
        </div>
      );

    case 'select':
      return (
        <FieldShell label={label} help={help} required={field.required}>
          <Select
            value={value || ''}
            onChange={onChange}
            options={(field.options ?? []).map((opt) => ({
              value: opt.value,
              label: resolveOptionLabel(t, opt),
            }))}
            placeholder={placeholder ?? label}
            searchable={false}
            ariaLabel={label}
          />
        </FieldShell>
      );

    case 'multiselect':
      return (
        <FieldShell label={label} help={help} required={field.required}>
          <Input
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder ?? `${(field.options ?? []).map((o) => o.value).join(', ')}`}
          />
        </FieldShell>
      );

    case 'string':
    default:
      return (
        <FieldShell label={label} help={help} required={field.required}>
          <Input
            type="text"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={placeholder ?? label}
          />
        </FieldShell>
      );
  }
}

function FieldShell({
  label,
  help,
  required,
  children,
}: {
  label: string;
  help?: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <Label className="mb-1">
        {label}
        {required && <span className="ml-0.5 text-destructive">*</span>}
      </Label>
      {children}
      {help && <p className="mt-1 text-[11px] text-muted-foreground">{help}</p>}
    </div>
  );
}

/** Option labels may be i18n keys prefixed with "@" — resolve those
 *  via react-i18next; otherwise return the literal label string. */
function resolveOptionLabel(
  t: (key: string, opts?: { defaultValue?: string }) => string,
  opt: ConfigFieldOption,
): string {
  if (opt.label.startsWith('@')) {
    const i18nKey = opt.label.slice(1);
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: schema-declared option labels reference plugin i18n keys that the host cannot enumerate
    return t(i18nKey, { defaultValue: opt.label });
  }
  return opt.label;
}
