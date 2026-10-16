// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { CORE_WIDGET_META } from '@resources/widgets/catalog-core';
import { MONITORING_WIDGET_META } from '@resources/widgets/catalog-monitoring';
import { OPERATIONS_WIDGET_META } from '@resources/widgets/catalog-operations';
import type { WidgetCategory, WidgetDefinitionMeta, WidgetTypeId } from '@resources/widgets/catalog-types';

export type { WidgetTypeId, WidgetCategory, WidgetDefinitionMeta, WidgetProps } from '@resources/widgets/catalog-types';

export const WIDGET_CATEGORIES: { id: WidgetCategory; labelKey: string }[] = [
  { id: 'resources', labelKey: 'categories.resources' },
  { id: 'host', labelKey: 'categories.host' },
  { id: 'metrics', labelKey: 'categories.metrics' },
  { id: 'monitoring', labelKey: 'categories.monitoring' },
  { id: 'operations', labelKey: 'categories.operations' },
  // Filled by PluginWidgetBridge from installed plugins' RegisterWidgets.
  { id: 'plugin', labelKey: 'categories.plugin' },
];

export const BUILTIN_WIDGET_META = {
  ...CORE_WIDGET_META,
  ...MONITORING_WIDGET_META,
  ...OPERATIONS_WIDGET_META,
} as Record<WidgetTypeId, WidgetDefinitionMeta>;

export const BUILTIN_WIDGET_DEFINITIONS = Object.values(BUILTIN_WIDGET_META);

export function getWidgetMeta(typeId: string): WidgetDefinitionMeta | undefined {
  return BUILTIN_WIDGET_META[typeId as WidgetTypeId];
}
