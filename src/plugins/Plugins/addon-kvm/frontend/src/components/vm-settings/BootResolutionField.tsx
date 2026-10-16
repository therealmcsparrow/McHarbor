// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import {
  BOOT_RESOLUTION_LIMITS,
  BOOT_RESOLUTION_PRESETS,
  CUSTOM_CHOICE,
  DEFAULT_CHOICE,
  adapterTakesResolution,
  isValidResolution,
  presetValue,
  resolutionChoice,
  resolutionFromChoice,
} from '../../bootResolution';
import type { Draft } from './settingsDraft';

export function BootResolutionField({
  draft,
  locked,
  set,
}: {
  draft: Draft;
  locked: boolean;
  set: <K extends keyof Draft>(field: K, value: Draft[K]) => void;
}) {
  const { t } = useTranslation('vm');
  const stored = resolutionChoice(draft.displayWidth, draft.displayHeight);
  // Custom stays open while the operator types, even when the digits briefly
  // spell out a preset.
  const [customOpen, setCustomOpen] = useState(stored === CUSTOM_CHOICE);
  const choice = customOpen ? CUSTOM_CHOICE : stored;
  const supported = adapterTakesResolution(draft.displayType);
  const valid = isValidResolution(draft.displayWidth, draft.displayHeight);

  const choose = (value: string) => {
    const size = resolutionFromChoice(value);
    if (!size) {
      setCustomOpen(true);
      return;
    }
    setCustomOpen(false);
    set('displayWidth', size.width);
    set('displayHeight', size.height);
  };

  let hint: string | undefined;
  if (!supported) hint = t('settings.fields.bootResolutionUnsupported');
  else if (!valid) hint = t('settings.fields.bootResolutionRange', BOOT_RESOLUTION_LIMITS);

  return (
    <SettingsField
      label={t('settings.fields.bootResolution')}
      description={t('settings.fields.bootResolutionHint')}
      hint={hint}
    >
      <Select
        value={choice}
        ariaLabel={t('settings.fields.bootResolution')}
        disabled={locked || !supported}
        options={[
          { value: DEFAULT_CHOICE, label: t('settings.fields.bootResolutionDefault') },
          ...BOOT_RESOLUTION_PRESETS.map((preset) => ({
            value: presetValue(preset),
            label: `${preset.width} × ${preset.height}`,
          })),
          { value: CUSTOM_CHOICE, label: t('settings.fields.bootResolutionCustom') },
        ]}
        onChange={choose}
      />
      {choice === CUSTOM_CHOICE && (
        <div className="mt-2 grid grid-cols-2 gap-2">
          <NumberInput
            nullable
            min={BOOT_RESOLUTION_LIMITS.minWidth}
            max={BOOT_RESOLUTION_LIMITS.maxWidth}
            value={draft.displayWidth || null}
            disabled={locked || !supported}
            aria-label={t('settings.fields.bootResolutionWidth')}
            placeholder={t('settings.fields.bootResolutionWidth')}
            onChange={(value) => set('displayWidth', value ?? 0)}
          />
          <NumberInput
            nullable
            min={BOOT_RESOLUTION_LIMITS.minHeight}
            max={BOOT_RESOLUTION_LIMITS.maxHeight}
            value={draft.displayHeight || null}
            disabled={locked || !supported}
            aria-label={t('settings.fields.bootResolutionHeight')}
            placeholder={t('settings.fields.bootResolutionHeight')}
            onChange={(value) => set('displayHeight', value ?? 0)}
          />
        </div>
      )}
    </SettingsField>
  );
}
