// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Switch } from '@resources/components/ui/Switch';
import { SettingsField } from '@resources/components/settings/SettingsGroup';

/**
 * A switch whose host cannot necessarily drive it.
 *
 * Disabled with the missing capability named, never hidden: an operator who
 * cannot find a setting assumes McHarbor does not have it, while one who sees
 * it greyed out with a reason knows what to change on the host.
 */
export function FeatureSwitch({
  label,
  description,
  checked,
  feature,
  disabled,
  onChange,
}: {
  label: string;
  description: string;
  checked: boolean;
  feature: { supported: boolean; reason: string };
  disabled: boolean;
  onChange: (value: boolean) => void;
}) {
  return (
    <SettingsField label={label} description={feature.supported ? description : feature.reason}>
      <Switch
        checked={checked}
        disabled={disabled || !feature.supported}
        aria-label={label}
        onCheckedChange={onChange}
      />
    </SettingsField>
  );
}
