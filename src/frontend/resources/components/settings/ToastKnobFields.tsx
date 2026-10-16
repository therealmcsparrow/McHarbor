// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBellRinging } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import { ToastPositionPicker } from '@resources/components/settings/ToastPositionPicker';
import { useToastPreviewRunner, type ToastKnobs } from '@resources/components/settings/useToastPreviewRunner';

export type { ToastKnobs } from '@resources/components/settings/useToastPreviewRunner';

type ToastKnobFieldsProps = {
  value: ToastKnobs;
  onChange: (next: ToastKnobs) => void;
  /** Greys the whole set out — used when a user has not opted to override. */
  disabled?: boolean;
  /** Permission key the controls gate on; omit for the per-user copy. */
  permission?: string;
  /** Distinguishes the two copies' element ids on the same page. */
  idPrefix: string;
};

// Duration is stored in milliseconds and edited in seconds: nobody thinks
// "4000" about a toast. 60s is the ceiling because past that the honest
// answer is 0, which means never.
const MAX_SECONDS = 60;

/**
 * The knobs that describe how a toast looks and behaves, rendered identically
 * for the app-wide default and for a user's own override.
 *
 * Sharing the fields is what makes the two cards legible next to each other:
 * a user comparing their override with the default is looking at the same
 * five controls in the same order, so the only difference on screen is the
 * values. The preview itself lives in `useToastPreviewRunner`.
 */
export function ToastKnobFields({ value, onChange, disabled, permission, idPrefix }: ToastKnobFieldsProps) {
  const { t } = useTranslation('settings');
  const handlePreview = useToastPreviewRunner(value);
  const neverDismisses = value.durationMs === 0;

  return (
    <>
      <SettingsField label={t('notifications.positionLabel')} description={t('notifications.positionDescription')}>
        <ToastPositionPicker
          value={value.position}
          onChange={(position) => onChange({ ...value, position })}
          disabled={disabled}
          permission={permission}
          idPrefix={idPrefix}
        />
      </SettingsField>

      <div className="grid gap-5 lg:grid-cols-2">
        <SettingsField
          htmlFor="settings-toast-knob-fields-2"
          label={t('notifications.durationLabel')}
          description={neverDismisses ? t('notifications.durationNever') : t('notifications.durationDescription')}
        >
          <NumberInput
            id="settings-toast-knob-fields-2"
            value={Math.round(value.durationMs / 1000)}
            onChange={(seconds) => onChange({ ...value, durationMs: seconds * 1000 })}
            min={0}
            max={MAX_SECONDS}
            disabled={disabled}
            className="w-40"
          />
        </SettingsField>

        <SettingsField
          htmlFor="settings-toast-knob-fields-3"
          label={t('notifications.maxVisibleLabel')}
          description={t('notifications.maxVisibleDescription')}
        >
          <NumberInput
            id="settings-toast-knob-fields-3"
            value={value.maxVisible}
            onChange={(maxVisible) => onChange({ ...value, maxVisible })}
            min={1}
            max={10}
            disabled={disabled}
            className="w-40"
          />
        </SettingsField>

        <SettingsField
          htmlFor={`${idPrefix}-rich-colors`}
          label={t('notifications.richColorsLabel')}
          description={t('notifications.richColorsDescription')}
        >
          <Switch
            id={`${idPrefix}-rich-colors`}
            checked={value.richColors}
            onCheckedChange={(richColors) => onChange({ ...value, richColors })}
            disabled={disabled}
            aria-label={t('notifications.richColorsLabel')}
          />
        </SettingsField>

        <SettingsField
          htmlFor={`${idPrefix}-close-button`}
          label={t('notifications.closeButtonLabel')}
          description={
            neverDismisses ? t('notifications.closeButtonForced') : t('notifications.closeButtonDescription')
          }
        >
          {/* Forced on when nothing auto-dismisses: without it the toast has
              no way to leave the screen, and the backend rejects that pairing
              anyway. Showing it locked on is more honest than letting the
              save fail. */}
          <Switch
            id={`${idPrefix}-close-button`}
            checked={neverDismisses ? true : value.closeButton}
            onCheckedChange={(closeButton) => onChange({ ...value, closeButton })}
            disabled={disabled || neverDismisses}
            aria-label={t('notifications.closeButtonLabel')}
          />
        </SettingsField>
      </div>

      <SettingsField label={t('notifications.previewLabel')} description={t('notifications.previewDescription')}>
        {/* Not permission-gated: raising a sample toast changes nothing, and a
            read-only user is exactly who benefits from seeing what the
            operator's default actually looks like. */}
        <Button type="button" variant="outline" size="sm" onClick={handlePreview}>
          <IconBellRinging className="size-4" />
          {t('notifications.previewAction')}
        </Button>
      </SettingsField>
    </>
  );
}
