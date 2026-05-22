// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AlertSeverity, AlertType } from '../hooks/useAlerts';

export const ALERT_SEVERITIES: AlertSeverity[] = ['critical', 'warning', 'info'];

export const ALERT_TYPES: AlertType[] = [
  'cpu',
  'memory',
  'disk',
  'container_down',
  'image_update',
];

export const ALERT_SEVERITY_LABEL_KEYS: Record<AlertSeverity, string> = {
  critical: 'alerts.severityCritical',
  warning: 'alerts.severityWarning',
  info: 'alerts.severityInfo',
};

export const ALERT_TYPE_LABEL_KEYS: Record<AlertType, string> = {
  cpu: 'alerts.typeCpu',
  memory: 'alerts.typeMemory',
  disk: 'alerts.typeDisk',
  container_down: 'alerts.typeContainerDown',
  image_update: 'alerts.typeImageUpdate',
};

export const ALERT_SEVERITY_BADGE_VARIANTS: Record<AlertSeverity, 'default' | 'warning' | 'destructive'> = {
  critical: 'destructive',
  warning: 'warning',
  info: 'default',
};
