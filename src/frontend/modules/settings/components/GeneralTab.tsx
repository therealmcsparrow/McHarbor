// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { AutoRefreshCard, DateTimeFormatCard } from './GeneralDisplayCards';
import { ApplicationLanguageCard, TimezoneCard } from './GeneralLocalizationCards';
import type { AppSettings } from '../types';

type GeneralTabProps = {
  settings?: AppSettings;
};

/**
 * Settings → General.
 *
 * The tab is a stack of self-contained groups, ordered from "how the app
 * speaks to everyone" down to "how it speaks to me". Each group states who
 * it affects and owns exactly one Save button, so no setting on this page is
 * ambiguous about scope or about which button writes it.
 *
 *   1. Application language  — app-wide default locale
 *   2. Timezone              — app-wide, recreates the container
 *   3. Date & time display   — the signed-in user's own formats
 *   4. Auto-refresh          — app-wide polling interval
 *
 * Data retention and the notification defaults used to close this list.
 * Each now has a section of its own — see `RetentionTab` under Monitoring
 * and `NotificationsTab` beside this one.
 */
export function GeneralTab({ settings }: GeneralTabProps) {
  return (
    <div className="space-y-6">
      <ApplicationLanguageCard />
      <TimezoneCard />
      <DateTimeFormatCard />
      <AutoRefreshCard settings={settings} />
    </div>
  );
}
