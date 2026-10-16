// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCalendarTime, IconRefresh } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { useAuth } from '@core/auth/useAuth';
import { useUpdatePreferences } from '@core/auth/usePreferences';
import { useSaveSettings, useSettings } from '../hooks/useSettings';
import type { AppSettings } from '../types';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { PERM } from '@core/auth/permissionKeys';

const DEFAULT_REFRESH_INTERVAL = 10;

/**
 * Per-user display preferences: time format (12h / 24h) and date format
 * (DD/MM/YYYY vs MM/DD/YYYY). These write to /auth/preferences, the same
 * endpoint that already handles preferredLanguage.
 */
export function DateTimeFormatCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const updatePreferences = useUpdatePreferences();

  const [timeFormat, setTimeFormat] = useState<'12h' | '24h'>(user?.timeFormat === '12h' ? '12h' : '24h');
  const [dateFormat, setDateFormat] = useState<'ddmmyyyy' | 'mmddyyyy'>(
    user?.dateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy',
  );

  // Keep local state in sync when the user record changes (login,
  // settings save, etc.) so the form reflects the latest server value.
  const userTimeFormat = user?.timeFormat;
  const userDateFormat = user?.dateFormat;
  const hasUser = Boolean(user);
  const userKey = `${hasUser}|${userTimeFormat}|${userDateFormat}`;
  const [syncedUserKey, setSyncedUserKey] = useState(userKey);
  if (userKey !== syncedUserKey) {
    setSyncedUserKey(userKey);
    if (hasUser) {
      setTimeFormat(userTimeFormat === '12h' ? '12h' : '24h');
      setDateFormat(userDateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy');
    }
  }

  const dirty = user?.timeFormat !== timeFormat || user?.dateFormat !== dateFormat;

  async function handleSave() {
    if (!user) return;
    try {
      await updatePreferences.mutateAsync({ timeFormat, dateFormat });
      toast.success(t('general.preferencesSaved'));
    } catch {
      // The preferences mutation reports its own failure through the global mutation cache.
    }
  }

  if (!user) return null;

  return (
    <SettingsGroup
      icon={IconCalendarTime}
      title={t('general.groups.display.title')}
      description={t('general.userInterfacePreferencesDescription')}
      scope={t('general.scopePersonal')}
      actions={
        <Button onClick={handleSave} disabled={!dirty || updatePreferences.isPending}>
          {updatePreferences.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      <div className="grid gap-5 sm:grid-cols-2">
        <SettingsField label={t('general.timeFormat')}>
          <Select
            value={timeFormat}
            onChange={(v) => setTimeFormat(v === '12h' ? '12h' : '24h')}
            options={[
              { value: '24h', label: t('general.timeFormat24h') },
              { value: '12h', label: t('general.timeFormat12h') },
            ]}
            className="max-w-sm"
            ariaLabel={t('general.timeFormat')}
          />
        </SettingsField>
        <SettingsField label={t('general.dateFormat')}>
          <Select
            value={dateFormat}
            onChange={(v) => setDateFormat(v === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy')}
            options={[
              { value: 'ddmmyyyy', label: t('general.dateFormatDDMMYYYY') },
              { value: 'mmddyyyy', label: t('general.dateFormatMMDDYYYY') },
            ]}
            className="max-w-sm"
            ariaLabel={t('general.dateFormat')}
          />
        </SettingsField>
      </div>
    </SettingsGroup>
  );
}

type AutoRefreshCardProps = {
  /** Optional pre-loaded settings; falls back to the settings query. */
  settings?: AppSettings;
};

/** App-wide polling interval for the live views. */
export function AutoRefreshCard({ settings }: AutoRefreshCardProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const settingsQuery = useSettings();
  const saveSettings = useSaveSettings();

  const serverInterval =
    settings?.autoRefreshInterval ?? settingsQuery.data?.autoRefreshInterval ?? DEFAULT_REFRESH_INTERVAL;
  const [refreshInterval, setRefreshInterval] = useState(serverInterval);

  // The settings query resolves after mount, so re-seed the input once
  // the server value arrives instead of leaving the default on screen.
  const [syncedInterval, setSyncedInterval] = useState(serverInterval);
  if (serverInterval !== syncedInterval) {
    setSyncedInterval(serverInterval);
    setRefreshInterval(serverInterval);
  }

  const dirty = serverInterval !== refreshInterval;

  return (
    <SettingsGroup
      icon={IconRefresh}
      title={t('general.groups.refresh.title')}
      description={t('general.groups.refresh.description')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => saveSettings.mutate({ autoRefreshInterval: refreshInterval })}
          disabled={!dirty || saveSettings.isPending}
        >
          {saveSettings.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      <SettingsField
        label={t('general.autoRefreshInterval')}
        description={t('general.autoRefreshIntervalDescription')}
        htmlFor="settings-auto-refresh-interval"
      >
        <div className="flex items-center gap-2">
          <NumberInput
            id="settings-auto-refresh-interval"
            value={refreshInterval}
            onChange={setRefreshInterval}
            min={1}
            max={300}
            className="w-40"
          />
          <span className="text-xs text-muted-foreground">{t('general.seconds')}</span>
        </div>
      </SettingsField>
    </SettingsGroup>
  );
}
