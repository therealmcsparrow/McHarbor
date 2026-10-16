// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Text as UiText } from '@resources/components/ui/Typography';
import type { PolicyForm } from '@resources/hooks/usePasswordPolicySettings';

type ToggleKey = Extract<
  keyof PolicyForm,
  'password_require_upper' | 'password_require_lower' | 'password_require_digit' | 'password_require_symbol'
>;

const TOGGLES: Array<{ key: ToggleKey; id: string; labelKey: string }> = [
  {
    key: 'password_require_upper',
    id: 'password-require-upper',
    labelKey: 'security.passwordPolicy.requireUpper',
  },
  {
    key: 'password_require_lower',
    id: 'password-require-lower',
    labelKey: 'security.passwordPolicy.requireLower',
  },
  {
    key: 'password_require_digit',
    id: 'password-require-digit',
    labelKey: 'security.passwordPolicy.requireDigit',
  },
  {
    key: 'password_require_symbol',
    id: 'password-require-symbol',
    labelKey: 'security.passwordPolicy.requireSymbol',
  },
];

type PasswordPolicyClassTogglesProps = {
  form: PolicyForm;
  disabled: boolean;
  onChange: (key: ToggleKey, checked: boolean) => void;
};

/** The four character-class requirements enforced on every new password. */
export function PasswordPolicyClassToggles({ form, disabled, onChange }: PasswordPolicyClassTogglesProps) {
  const { t } = useTranslation('settings');

  return (
    <div className="space-y-2">
      <UiText size="sm" weight="medium">
        {t('security.passwordPolicy.characterClasses')}
      </UiText>
      <Stack gap={2}>
        {TOGGLES.map((toggle) => (
          <Flex key={toggle.key} align="center" justify="between" className="rounded-md border border-border p-2.5">
            <Label htmlFor={toggle.id} className="text-xs">
              {t(toggle.labelKey)}
            </Label>
            <Switch
              id={toggle.id}
              checked={form[toggle.key]}
              onCheckedChange={(checked) => onChange(toggle.key, checked)}
              disabled={disabled}
            />
          </Flex>
        ))}
      </Stack>
    </div>
  );
}
