// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The three faces of one API key.
//
// Same model as the user detail page, one destination shorter. The *tab id*
// is the unit of navigation, so `?tab=<id>` on `/security/api-keys/:id`
// resolves directly and a link to a key's history is a link, not a scroll
// position.
//
//   1. What is this credential?     -> Overview
//   2. What is it allowed to do?    -> Access
//   3. What has it actually done?   -> Usage

import { IconInfoCircle, IconShieldLock, IconChartHistogram, type TablerIcon } from '@tabler/icons-react';

export const apiKeyDetailTabIDs = ['overview', 'access', 'usage'] as const;

export type APIKeyDetailTabId = (typeof apiKeyDetailTabIDs)[number];

export function isAPIKeyDetailTabId(value: string | null): value is APIKeyDetailTabId {
  return apiKeyDetailTabIDs.some((tabID) => tabID === value);
}

export const apiKeyDetailTabIcons: Record<APIKeyDetailTabId, TablerIcon> = {
  overview: IconInfoCircle,
  access: IconShieldLock,
  usage: IconChartHistogram,
};

/** i18n keys in the `security` namespace. */
export const apiKeyDetailTabLabelKeys: Record<APIKeyDetailTabId, string> = {
  overview: 'apiKeys.detail.tabs.overview',
  access: 'apiKeys.detail.tabs.access',
  usage: 'apiKeys.detail.tabs.usage',
};
