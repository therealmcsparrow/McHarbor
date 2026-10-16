// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import type { TargetSelectOption } from '../hooks/useRunbookTargets';

const MANUAL = '__manual__';

type RunbookPickFieldProps = {
  value: string;
  onChange: (value: string) => void;
  options: TargetSelectOption[];
  ariaLabel: string;
  placeholder: string;
  emptyOption?: TargetSelectOption;
  allowManual?: boolean;
  disabled?: boolean;
};

export function RunbookPickField({
  value,
  onChange,
  options,
  ariaLabel,
  placeholder,
  emptyOption,
  allowManual = true,
  disabled,
}: RunbookPickFieldProps) {
  const { t } = useTranslation('runbooks');
  const known = value === '' || options.some((option) => option.value === value);
  const [manual, setManual] = useState(!known);

  if (manual && allowManual) {
    return (
      <div className="flex min-w-0 gap-2">
        <Input
          value={value}
          onChange={(evt) => onChange(evt.target.value)}
          placeholder={t('editor.manualPlaceholder')}
          aria-label={ariaLabel}
          disabled={disabled}
        />
        <Button type="button" variant="outline" disabled={disabled} onClick={() => setManual(false)}>
          {t('editor.fromList')}
        </Button>
      </div>
    );
  }

  const choices = [
    ...(emptyOption ? [{ ...emptyOption, value: '' }] : []),
    ...options,
    ...(allowManual ? [{ value: MANUAL, label: t('editor.manual') }] : []),
  ];
  return (
    <Select
      ariaLabel={ariaLabel}
      placeholder={placeholder}
      value={known ? value : ''}
      disabled={disabled}
      options={choices}
      onChange={(next) => {
        if (next === MANUAL) {
          setManual(true);
          onChange('');
          return;
        }
        onChange(next);
      }}
    />
  );
}
