// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { zones1 } from './timezoneData/zones1';
import { zones2 } from './timezoneData/zones2';
import { zones3 } from './timezoneData/zones3';

const bundledTimezones = [zones1, zones2, zones3].join('\n');

export const SUPPORTED_TIMEZONES = bundledTimezones.split('\n');

const supportedTimezoneSet = new Set(SUPPORTED_TIMEZONES);

export function isSupportedTimezone(value: string | null | undefined): value is string {
  if (typeof value !== 'string') {
    return false;
  }

  const trimmed = value.trim();
  return trimmed.length > 0 && supportedTimezoneSet.has(trimmed);
}

export function normalizeTimezone(value: string | null | undefined): string {
  if (!isSupportedTimezone(value)) {
    return 'UTC';
  }

  return value.trim();
}
