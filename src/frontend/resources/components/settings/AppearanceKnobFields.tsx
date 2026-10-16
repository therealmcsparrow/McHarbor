// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// i18n keys added by this file:
//   settings:appearance.{themeLabel,themeDescription,accentLabel,accentDescription}
//   settings:appearance.{fontLabel,fontDescription,fontPreview,fontOnlineHint}
//   settings:appearance.{baseFontLabel,baseFontDescription,baseFontUnit,baseFontDerived,baseFontReset}
//   settings:appearance.accentTheme
//   settings:appearance.accent.{blue,green,purple,orange,rose,cyan}
//   settings:appearance.font.<id> for every id in FONT_CHOICES (26)

import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { cssVars } from '@resources/utils/cssVars';
import { usePluginThemes } from '@core/plugins/loader';
import { FONT_CHOICES, FONT_STACKS, isGoogleFont, type ThemeFont } from '@resources/stores/themeCustomizer';
import { ensureGoogleFont } from '@resources/theme/google-fonts';
import { getAllThemes } from '@/src/themes';
import type { AppearanceSettings } from '@resources/types/settings';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import { AppearanceAccentRow } from '@resources/components/settings/AppearanceAccentRow';
import { AppearanceBaseFontField } from '@resources/components/settings/AppearanceBaseFontField';

/** The four look knobs, without the operator-only allowUserOverride flag. */
export type AppearanceKnobs = Omit<AppearanceSettings, 'allowUserOverride'>;

type AppearanceKnobFieldsProps = {
  value: AppearanceKnobs;
  onChange: (next: AppearanceKnobs) => void;
  /** Greys the whole set out — used when a user has not opted to override. */
  disabled?: boolean;
  /** Permission key the controls gate on; omit for the per-user copy. */
  permission?: string;
  /** Distinguishes the two copies' element ids on the same page. */
  idPrefix: string;
  /**
   * What the base-size reset button goes back to. Not a constant, because the
   * two cards mean different things by "reset": the operator card is the
   * app-wide layer, so it resets to the shipped 16px; the personal card sits
   * on top of that layer, so it resets to whatever the operator set.
   */
  baseFontDefault: number;
};

/**
 * The four controls that describe how the app looks, rendered identically for
 * the app-wide default and for a user's own override.
 *
 * Sharing the fields is what makes the two cards legible next to each other:
 * a user comparing their override with the default is looking at the same
 * four controls in the same order, so the only difference on screen is the
 * values. Same reasoning — and same shape — as `ToastKnobFields`.
 *
 * The theme control is a plain select rather than a grid of swatch tiles. The
 * tiles were generated from a single hue in the manifest, so they showed a
 * colour wheel that no theme actually renders; a row of names is both smaller
 * and more honest.
 */
export function AppearanceKnobFields({
  value,
  onChange,
  disabled,
  permission,
  idPrefix,
  baseFontDefault,
}: AppearanceKnobFieldsProps) {
  const { t } = useTranslation('settings');
  // Loaded plugins register themes at runtime, so the list is re-read whenever
  // the plugin set changes rather than captured once at module load.
  const pluginThemes = usePluginThemes();
  const themeOptions = useMemo(() => {
    void pluginThemes;
    return getAllThemes().map((theme) => ({
      value: theme.id,
      label: theme.name,
    }));
  }, [pluginThemes]);

  const fontOptions = FONT_CHOICES.map((font) => ({
    value: font,
    label: t(`appearance.font.${font}`),
  }));

  // Fetch the *drafted* font, not the saved one. Without this the preview line
  // below would render the fallback until the form is saved, which makes the
  // preview useless for exactly the fonts it exists to preview.
  useEffect(() => {
    ensureGoogleFont(value.font);
  }, [value.font]);

  return (
    <>
      {/* Paired by what they do rather than by what fits: the palette on one
          row, the type on the next. */}
      <div className="grid gap-5 sm:grid-cols-2">
        <SettingsField label={t('appearance.themeLabel')} description={t('appearance.themeDescription')}>
          <Select
            value={value.themeId}
            onChange={(themeId) => onChange({ ...value, themeId })}
            options={themeOptions}
            disabled={disabled}
            ariaLabel={t('appearance.themeLabel')}
          />
        </SettingsField>

        <SettingsField label={t('appearance.accentLabel')} description={t('appearance.accentDescription')}>
          <AppearanceAccentRow
            accent={value.accent}
            onPick={(accent) => onChange({ ...value, accent })}
            disabled={disabled}
            permission={permission}
            idPrefix={idPrefix}
          />
        </SettingsField>
      </div>

      <div className="grid gap-5 sm:grid-cols-2">
        <SettingsField
          label={t('appearance.fontLabel')}
          description={t('appearance.fontDescription')}
          hint={
            <>
              <span
                className="[font-family:var(--font)] text-sm text-foreground"
                // The value being previewed is a runtime choice, so it travels
                // as a CSS variable the class reads.
                style={cssVars({ font: FONT_STACKS[value.font] })}
              >
                {t('appearance.fontPreview')}
              </span>
              {/* Said only for the families it is true of. Repeating it under
                  every font would train people to stop reading it. */}
              {isGoogleFont(value.font) && <span className="mt-1 block">{t('appearance.fontOnlineHint')}</span>}
            </>
          }
        >
          <Select
            value={value.font}
            onChange={(font) => onChange({ ...value, font: font as ThemeFont })}
            options={fontOptions}
            disabled={disabled}
            ariaLabel={t('appearance.fontLabel')}
          />
        </SettingsField>

        <AppearanceBaseFontField
          baseFontPx={value.baseFontPx}
          onChange={(baseFontPx) => onChange({ ...value, baseFontPx })}
          disabled={disabled}
          permission={permission}
          idPrefix={idPrefix}
          baseFontDefault={baseFontDefault}
        />
      </div>
    </>
  );
}
