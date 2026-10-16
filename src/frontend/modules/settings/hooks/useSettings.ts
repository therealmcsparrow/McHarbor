// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AppSettings, TimezoneSettingsData } from '../types';

/** A single row as the settings table stores it and `GET /settings` returns it. */
type SettingRow = {
  id: string;
  key: string;
  value: string;
  category: string;
  updatedAt: string;
};

/**
 * The backend persists settings as key/value rows, not as an `AppSettings`
 * object: `GET /settings` answers `SettingRow[]` and `PUT /settings` takes
 * `{ settings: [{ key, value, category }] }`. This table is the single place
 * that maps the typed fields onto those row keys, so both directions stay in
 * step with the schema.
 */
const SETTING_KEYS = {
  appName: 'app_name',
  sessionTimeout: 'session_timeout',
  autoRefreshInterval: 'auto_refresh_interval',
  enableRegistration: 'enable_registration',
  defaultEnvironment: 'default_environment',
  defaultLanguage: 'default_language',
} as const satisfies Record<keyof AppSettings, string>;

const SETTINGS_CATEGORY = 'general';

const DEFAULT_SETTINGS: AppSettings = {
  appName: 'McHarbor',
  sessionTimeout: 86_400,
  autoRefreshInterval: 10,
  enableRegistration: false,
  defaultEnvironment: '',
  defaultLanguage: 'en',
};

function readNumber(raw: string | undefined, fallback: number): number {
  if (raw === undefined || raw.trim() === '') return fallback;
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function rowsToSettings(rows: SettingRow[]): AppSettings {
  const byKey = new Map(rows.map((row) => [row.key, row.value]));
  const read = (field: keyof AppSettings) => byKey.get(SETTING_KEYS[field]);
  const enableRegistration = read('enableRegistration');

  return {
    appName: read('appName') ?? DEFAULT_SETTINGS.appName,
    sessionTimeout: readNumber(read('sessionTimeout'), DEFAULT_SETTINGS.sessionTimeout),
    autoRefreshInterval: readNumber(read('autoRefreshInterval'), DEFAULT_SETTINGS.autoRefreshInterval),
    enableRegistration:
      enableRegistration === undefined
        ? DEFAULT_SETTINGS.enableRegistration
        : enableRegistration === 'true' || enableRegistration === '1',
    defaultEnvironment: read('defaultEnvironment') ?? DEFAULT_SETTINGS.defaultEnvironment,
    defaultLanguage: read('defaultLanguage') ?? DEFAULT_SETTINGS.defaultLanguage,
  };
}

function settingsToRows(settings: Partial<AppSettings>) {
  return (Object.keys(SETTING_KEYS) as (keyof AppSettings)[])
    .filter((field) => settings[field] !== undefined)
    .map((field) => ({
      key: SETTING_KEYS[field],
      value: String(settings[field]),
      category: SETTINGS_CATEGORY,
    }));
}

export function useSettings() {
  return useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get<SettingRow[]>('/settings').then((r) => rowsToSettings(r.data ?? [])),
  });
}

export function useSaveSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (settings: Partial<AppSettings>) =>
      api.put('/settings', { settings: settingsToRows(settings) }).then(assertSuccess),
    meta: { success: t('toast.settingsSaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
}

export function useTimezoneSettings() {
  return useQuery({
    queryKey: ['settings', 'timezone'],
    queryFn: () => api.get<TimezoneSettingsData>('/settings/timezone').then(assertSuccess),
  });
}

export function useSaveTimezoneSettings() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (timezone: string) =>
      runWithMcHarborRestart(() =>
        api.put<TimezoneSettingsData>('/settings/timezone', { timezone }).then(assertSuccess),
      ),
    meta: { success: t('general.timezoneSaveScheduled'), error: t('general.timezoneSaveFailed') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings', 'timezone'] });
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
}
