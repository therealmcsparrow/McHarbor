// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useAuth } from '@core/auth/useAuth';
import type { LocaleFormat } from '../utils/format';

const DEFAULT: LocaleFormat = {
  timeFormat: '24h',
  dateFormat: 'ddmmyyyy',
};

// useLocaleFormat returns the active user's time + date format
// preferences. Reads from the auth store on every render so the
// Settings - General page reflects the user's picks the moment the
// Save mutation succeeds. Falls back to the McHarbor defaults
// (24h + ddmmyyyy) when the auth store is empty (login screen,
// pre-hydration).
export function useLocaleFormat(): LocaleFormat {
  const user = useAuth((s) => s.user);
  if (!user) return DEFAULT;
  return {
    timeFormat: user.timeFormat === '12h' ? '12h' : '24h',
    dateFormat: user.dateFormat === 'mmddyyyy' ? 'mmddyyyy' : 'ddmmyyyy',
  };
}
