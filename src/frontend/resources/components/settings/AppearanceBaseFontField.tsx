// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconRotate } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { MAX_BASE_FONT_PX, MIN_BASE_FONT_PX, clampBaseFontPx } from '@resources/stores/themeCustomizer';
import { SettingsField } from '@resources/components/settings/SettingsGroup';

/**
 * Tailwind's own steps, as multipliers of the root size. Shown next to the
 * base-size field so the number stops being abstract: someone choosing 16
 * can see that it makes body copy 14 and a page title 24.
 */
const DERIVED_STEPS: readonly { key: string; rem: number }[] = [
  { key: 'sm', rem: 0.875 },
  { key: 'base', rem: 1 },
  { key: 'lg', rem: 1.125 },
  { key: '2xl', rem: 1.5 },
];

type AppearanceBaseFontFieldProps = {
  baseFontPx: number;
  onChange: (px: number) => void;
  disabled?: boolean;
  permission?: string;
  idPrefix: string;
  /**
   * What the reset button goes back to. Not a constant, because the two
   * cards mean different things by "reset": the operator card is the
   * app-wide layer, so it resets to the shipped 16px; the personal card sits
   * on top of that layer, so it resets to whatever the operator set.
   */
  baseFontDefault: number;
};

/** The root font-size knob with its derived-sizes hint and reset button. */
export function AppearanceBaseFontField({
  baseFontPx,
  onChange,
  disabled,
  permission,
  idPrefix,
  baseFontDefault,
}: AppearanceBaseFontFieldProps) {
  const { t } = useTranslation('settings');
  return (
    <SettingsField
      htmlFor={`${idPrefix}-base-font`}
      label={t('appearance.baseFontLabel')}
      description={t('appearance.baseFontDescription')}
      hint={t('appearance.baseFontDerived', {
        steps: DERIVED_STEPS.map((step) => `${step.key} ${Math.round(baseFontPx * step.rem)}px`).join(' · '),
      })}
    >
      <div className="flex items-center gap-2">
        <NumberInput
          id={`${idPrefix}-base-font`}
          value={baseFontPx}
          onChange={(px) => onChange(clampBaseFontPx(px))}
          min={MIN_BASE_FONT_PX}
          max={MAX_BASE_FONT_PX}
          step={1}
          disabled={disabled}
          className="w-28"
          aria-label={t('appearance.baseFontLabel')}
        />
        <span className="text-xs text-muted-foreground">{t('appearance.baseFontUnit')}</span>
        {/* Names its target rather than saying just "Reset": on the
            personal card that target is the operator's number, which the
            user has no other way to see once they have overridden it.
            Inert while already on target, so it never claims work it
            would not do. */}
        <Button
          type="button"
          permission={permission}
          variant="ghost"
          size="sm"
          disabled={disabled || baseFontPx === baseFontDefault}
          onClick={() => onChange(baseFontDefault)}
          aria-label={t('appearance.baseFontReset', { px: baseFontDefault })}
        >
          <IconRotate className="size-3.5" />
          {t('appearance.baseFontReset', { px: baseFontDefault })}
        </Button>
      </div>
    </SettingsField>
  );
}
