// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';

const MANUAL = '__manual__';

type Option = { value: string; label: string; description?: string };

type Props = {
  value: string;
  onChange: (value: string) => void;
  options: Option[];
  ariaLabel: string;
  placeholder: string;
  manualPlaceholder: string;
};

export function PickOrType({ value, onChange, options, ariaLabel, placeholder, manualPlaceholder }: Props) {
  const { t } = useTranslation('reconciler');
  const known = options.some((o) => o.value === value);
  const [manual, setManual] = useState(value !== '' && !known);

  if (manual) {
    return (
      <div className="flex gap-2">
        <Input
          type="text"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={manualPlaceholder}
          aria-label={ariaLabel}
          ref={(el) => el?.focus()}
        />
        <Button type="button" variant="outline" onClick={() => setManual(false)}>
          {t('pick.fromList')}
        </Button>
      </div>
    );
  }
  return (
    <Select
      value={known ? value : ''}
      onChange={(next) => {
        if (next === MANUAL) {
          setManual(true);
          onChange('');
          return;
        }
        onChange(next);
      }}
      ariaLabel={ariaLabel}
      placeholder={placeholder}
      searchable
      options={[...options, { value: MANUAL, label: t('pick.manual') }]}
    />
  );
}
