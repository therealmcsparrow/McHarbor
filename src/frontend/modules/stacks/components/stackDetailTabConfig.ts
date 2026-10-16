// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconInfoCircle,
  IconBox,
  IconLayersSubtract,
  IconFileText,
  IconCode,
  IconVariable,
  IconWebhook,
} from '@tabler/icons-react';

export const DETAIL_TAB_IDS = ['overview', 'services', 'layers', 'logs', 'compose', 'environment', 'webhooks'] as const;
export type DetailTabId = (typeof DETAIL_TAB_IDS)[number];

export const DETAIL_TAB_ICONS: Record<DetailTabId, typeof IconInfoCircle> = {
  overview: IconInfoCircle,
  services: IconBox,
  layers: IconLayersSubtract,
  logs: IconFileText,
  compose: IconCode,
  environment: IconVariable,
  webhooks: IconWebhook,
};
