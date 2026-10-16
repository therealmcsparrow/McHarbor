// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { IconClockHour4, IconLanguage } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Select } from '@resources/components/ui/Select';
import { supportedLanguages, languageLabels, type SupportedLanguage } from '@core/i18n';
import { SUPPORTED_TIMEZONES } from '@resources/utils/timezones';
import { useSaveSettings, useSaveTimezoneSettings, useSettings, useTimezoneSettings } from '../hooks/useSettings';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { PERM } from '@core/auth/permissionKeys';

// Map a "default language" key stored on the server (which can be any
// string) to a SupportedLanguage for the <Select> value. Unknown values
// fall back to "en".
function normalizeDefaultLanguage(value: string | undefined | null): SupportedLanguage {
  if (!value) return 'en';
  if ((supportedLanguages as readonly string[]).includes(value)) {
    return value as SupportedLanguage;
  }
  return 'en';
}

/**
 * Admin-level app-wide default language. The backend stores it as a plain
 * settings row with key="default_language".
 */
export function ApplicationLanguageCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const settingsQuery = useSettings();
  const saveSettings = useSaveSettings();

  const serverLanguage = normalizeDefaultLanguage(settingsQuery.data?.defaultLanguage);
  const [defaultLanguage, setDefaultLanguage] = useState<SupportedLanguage>(serverLanguage);

  // Re-sync from the latest settings query so the <Select> reflects
  // server-side changes.
  const serverLanguageRaw = settingsQuery.data?.defaultLanguage;
  const [syncedLanguage, setSyncedLanguage] = useState(serverLanguageRaw);
  if (serverLanguageRaw !== syncedLanguage) {
    setSyncedLanguage(serverLanguageRaw);
    if (serverLanguageRaw) setDefaultLanguage(normalizeDefaultLanguage(serverLanguageRaw));
  }

  const dirty = serverLanguage !== defaultLanguage;

  function handleSave() {
    // Success and failure toasts come from the shared settings mutation
    // (`meta.success` and the global mutation-cache error handler).
    saveSettings.mutate({ defaultLanguage });
  }

  return (
    <SettingsGroup
      icon={IconLanguage}
      title={t('general.groups.language.title')}
      description={t('general.groups.language.description')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button permission={PERM.SETTINGS_MANAGE} onClick={handleSave} disabled={!dirty || saveSettings.isPending}>
          {saveSettings.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      <SettingsField
        label={t('general.defaultApplicationLanguage')}
        description={t('general.defaultApplicationLanguageDescription')}
      >
        <Select
          value={defaultLanguage}
          onChange={(v) => setDefaultLanguage(v as SupportedLanguage)}
          options={supportedLanguages.map((lang) => ({ value: lang, label: languageLabels[lang] }))}
          className="max-w-sm"
          ariaLabel={t('general.defaultApplicationLanguage')}
        />
      </SettingsField>
    </SettingsGroup>
  );
}

/**
 * Server-side timezone. Saving recreates the McHarbor container with the
 * selected TZ environment variable, so the Save button is confirmation-gated.
 */
export function TimezoneCard() {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const timezoneQuery = useTimezoneSettings();
  const saveTimezone = useSaveTimezoneSettings();

  const [timezone, setTimezone] = useState(timezoneQuery.data?.timezone ?? 'UTC');
  const [confirmOpen, setConfirmOpen] = useState(false);

  const serverTimezone = timezoneQuery.data?.timezone;
  const [syncedTimezone, setSyncedTimezone] = useState(serverTimezone);
  if (serverTimezone !== syncedTimezone) {
    setSyncedTimezone(serverTimezone);
    if (serverTimezone) setTimezone(serverTimezone);
  }

  const dirty = timezoneQuery.data?.timezone !== timezone;
  const options = SUPPORTED_TIMEZONES.includes(timezone) ? SUPPORTED_TIMEZONES : [timezone, ...SUPPORTED_TIMEZONES];

  async function handleSave() {
    try {
      await saveTimezone.mutateAsync(timezone);
      toast.success(t('general.timezoneSaveScheduled'));
      setConfirmOpen(false);
    } catch {
      // The timezone mutation reports its own failure through the global mutation cache.
    }
  }

  return (
    <SettingsGroup
      icon={IconClockHour4}
      title={t('general.groups.timezone.title')}
      description={t('general.groups.timezone.description')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => setConfirmOpen(true)}
          disabled={native || !timezoneQuery.data || !dirty || saveTimezone.isPending}
        >
          {saveTimezone.isPending ? t('general.saving') : t('general.saveTimezoneAndRestart')}
        </Button>
      }
    >
      <SettingsField
        label={t('general.timezone')}
        description={t('general.timezoneDescription')}
        hint={
          timezoneQuery.data?.runtimeTimezone
            ? t(native ? 'general.timezoneRuntimeNative' : 'general.timezoneRuntime', {
                timezone: timezoneQuery.data.runtimeTimezone,
              })
            : undefined
        }
      >
        <Select
          value={timezone}
          onChange={setTimezone}
          disabled={native}
          options={options.map((value) => ({ value, label: value }))}
          className="max-w-sm"
          ariaLabel={t('general.timezone')}
        />
      </SettingsField>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('general.timezoneConfirmTitle')}
        description={t('general.timezoneConfirmDescription', { timezone })}
        confirmLabel={t('general.saveTimezoneAndRestart')}
        loading={saveTimezone.isPending}
        onConfirm={handleSave}
      />
    </SettingsGroup>
  );
}
