// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Switch } from '@resources/components/ui/Switch';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import type { ScannerField } from '../hooks/useScanners';

type ScannerFormFieldsProps = {
  name: string;
  onNameChange: (name: string) => void;
  fields: ScannerField[];
  values: Record<string, string>;
  onValueChange: (key: string, value: string) => void;
  /** True once a save has been attempted; gates the red text. */
  showErrors: boolean;
  isMissing: (field: ScannerField) => boolean;
};

/**
 * The scanner form: a name, then whatever the adapter declared.
 *
 * The name is host-owned rather than part of any adapter's schema.
 * Several scanners can share an engine — a strict Trivy and a lenient
 * one, a production Clair and a staging one — and the name is the only
 * thing separating them in the list, so it is asked first and always
 * required.
 *
 * `bool` gets a real Switch rather than the text box the logging form
 * settles for. Half of what a scanner exposes is a flag ("ignore
 * unfixed", "only fixed"), and asking an operator to type `true` into a
 * security setting is how a scan silently reports the wrong thing.
 */
export function ScannerFormFields({
  name: itemName,
  onNameChange,
  fields,
  values,
  onValueChange,
  showErrors,
  isMissing,
}: ScannerFormFieldsProps) {
  const { t } = useTranslation('security');
  const nameMissing = showErrors && itemName.trim() === '';

  const toggles = fields.filter((field) => field.type === 'bool');
  const inputs = fields.filter((field) => field.type !== 'bool');

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="scan-cfg-name" className="mb-1 text-xs text-muted-foreground">
          {t('scanning.name')}
          <span className="ml-0.5 text-destructive" aria-hidden>
            *
          </span>
        </Label>
        <Input
          id="scan-cfg-name"
          variant="outline"
          value={itemName}
          onChange={(entry) => onNameChange(entry.target.value)}
          placeholder={t('scanning.namePlaceholder')}
        />
        {nameMissing && <p className="mt-1 text-xs text-destructive">{t('scanning.fieldRequired')}</p>}
      </div>

      {inputs.length > 0 && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {inputs.map((field) => (
            <div key={field.key}>
              <Label htmlFor={`scanner-${field.key}`} className="mb-1 text-xs text-muted-foreground">
                {field.label}
                {field.required && (
                  <span className="ml-0.5 text-destructive" aria-hidden>
                    *
                  </span>
                )}
              </Label>
              {field.type === 'secret' ? (
                <PasswordInput
                  id={`scanner-${field.key}`}
                  value={values[field.key] ?? ''}
                  onChange={(eventItem) => onValueChange(field.key, eventItem.target.value)}
                  placeholder={field.hasValue ? t('scanning.secretStored') : field.placeholder}
                />
              ) : field.type === 'int' ? (
                <NumberInput
                  id={`scanner-${field.key}`}
                  nullable
                  value={toNumberOrNull(values[field.key])}
                  onChange={(next) => onValueChange(field.key, toNumberText(next))}
                  placeholder={field.placeholder}
                />
              ) : (
                <Input
                  id={`scanner-${field.key}`}
                  variant="outline"
                  type="text"
                  value={values[field.key] ?? ''}
                  onChange={(evt) => onValueChange(field.key, evt.target.value)}
                  placeholder={field.placeholder}
                />
              )}
              {showErrors && isMissing(field) ? (
                <p className="mt-1 text-xs text-destructive">{t('scanning.fieldRequired')}</p>
              ) : (
                field.help && <p className="mt-1 text-xs text-muted-foreground">{field.help}</p>
              )}
            </div>
          ))}
        </div>
      )}

      {toggles.length > 0 && (
        <div className="space-y-3 border-t border-border pt-4">
          {toggles.map((field) => (
            <div key={field.key} className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <Label htmlFor={`scanner-${field.key}`} className="text-sm font-medium text-foreground">
                  {field.label}
                </Label>
                {field.help && <p className="mt-1 text-xs text-muted-foreground">{field.help}</p>}
              </div>
              <Switch
                id={`scanner-${field.key}`}
                checked={(values[field.key] ?? '') === 'true'}
                onCheckedChange={(checked) => onValueChange(field.key, checked ? 'true' : 'false')}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
