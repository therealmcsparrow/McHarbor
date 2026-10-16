// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { useSecrets } from '@resources/hooks/useSecrets';
import { secretReference } from '@core/types/secrets';

type SecretPickerProps = {
  /** The current field value, which may or may not be a ${secret:NAME} reference. */
  value: string;
  /** Called with the full `${secret:NAME}` reference text, not the bare name. */
  onChange: (reference: string) => void;
  disabled?: boolean;
  className?: string;
};

/**
 * Picks a vault secret for an environment-variable field.
 *
 * It emits the **reference** (`${secret:NAME}`), never a value — the picker
 * cannot read values and the list endpoint does not return them. The
 * substitution happens server-side at deploy time.
 */
export function SecretPicker({ value, onChange, disabled, className }: SecretPickerProps) {
  const { t } = useTranslation('secrets');
  const { data: secrets = [] } = useSecrets();

  const options = useMemo(
    () =>
      secrets.map((secret) => ({
        value: secretReference(secret.name),
        label: `${secret.name} · ${t(`scope.${secret.scope}`)}`,
      })),
    [secrets, t],
  );

  return (
    <Select
      className={className}
      value={options.some((option) => option.value === value) ? value : ''}
      onChange={onChange}
      options={options}
      disabled={disabled || options.length === 0}
      placeholder={options.length === 0 ? t('picker.empty') : t('picker.placeholder')}
      ariaLabel={t('picker.label')}
    />
  );
}
