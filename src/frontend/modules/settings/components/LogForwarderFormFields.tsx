// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import type { LogForwarderField } from '../hooks/useLogForwarders';

type LogForwarderFormFieldsProps = {
  name: string;
  onNameChange: (name: string) => void;
  fields: LogForwarderField[];
  values: Record<string, string>;
  onValueChange: (key: string, value: string) => void;
  /** True once a save has been attempted; gates the red text. */
  showErrors: boolean;
  isMissing: (field: LogForwarderField) => boolean;
};

/**
 * The forwarder form: a name, then whatever the adapter declared.
 *
 * The name is host-owned rather than part of any adapter's schema.
 * Several forwarders can share a type — two SIEM collectors, a local
 * and an off-site syslog — and the name is the only thing separating
 * them in the list, so it is asked first and always required.
 */
export function LogForwarderFormFields({
  name: forwarderName,
  onNameChange,
  fields,
  values,
  onValueChange,
  showErrors,
  isMissing,
}: LogForwarderFormFieldsProps) {
  const { t } = useTranslation('settings');
  const nameMissing = showErrors && forwarderName.trim() === '';

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="forwarder-name" className="mb-1 text-xs text-muted-foreground">
          {t('logging.name')}
          <span className="ml-0.5 text-destructive" aria-hidden>
            *
          </span>
        </Label>
        <Input
          id="forwarder-name"
          variant="outline"
          value={forwarderName}
          onChange={(changeEvent) => onNameChange(changeEvent.target.value)}
          placeholder={t('logging.namePlaceholder')}
        />
        {nameMissing && <p className="mt-1 text-xs text-destructive">{t('logging.fieldRequired')}</p>}
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {fields.map((field) => (
          <div key={field.key}>
            <Label htmlFor={`forwarder-${field.key}`} className="mb-1 text-xs text-muted-foreground">
              {field.label}
              {field.required && (
                <span className="ml-0.5 text-destructive" aria-hidden>
                  *
                </span>
              )}
            </Label>
            {field.type === 'secret' ? (
              <PasswordInput
                id={`forwarder-${field.key}`}
                value={values[field.key] ?? ''}
                onChange={(changeEvent) => onValueChange(field.key, changeEvent.target.value)}
                placeholder={field.hasValue ? t('logging.secretStored') : field.placeholder}
              />
            ) : field.type === 'int' ? (
              <NumberInput
                id={`forwarder-${field.key}`}
                nullable
                value={toNumberOrNull(values[field.key])}
                onChange={(next) => onValueChange(field.key, toNumberText(next))}
                placeholder={field.placeholder}
              />
            ) : (
              <Input
                id={`forwarder-${field.key}`}
                variant="outline"
                type="text"
                value={values[field.key] ?? ''}
                onChange={(changeEvent) => onValueChange(field.key, changeEvent.target.value)}
                placeholder={field.placeholder}
              />
            )}
            {showErrors && isMissing(field) ? (
              <p className="mt-1 text-xs text-destructive">{t('logging.fieldRequired')}</p>
            ) : (
              field.help && <p className="mt-1 text-xs text-muted-foreground">{field.help}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
