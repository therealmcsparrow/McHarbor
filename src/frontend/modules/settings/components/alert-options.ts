// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AlertSeverity } from '@resources/hooks/useAlerts';

export const ALERT_SEVERITIES: AlertSeverity[] = ['critical', 'warning', 'info'];

export const ALERT_SEVERITY_LABEL_KEYS: Record<AlertSeverity, string> = {
  critical: 'alerts.severityCritical',
  warning: 'alerts.severityWarning',
  info: 'alerts.severityInfo',
};

export const ALERT_SEVERITY_BADGE_VARIANTS: Record<AlertSeverity, 'default' | 'warning' | 'destructive'> = {
  critical: 'destructive',
  warning: 'warning',
  info: 'default',
};

/**
 * Alert types are no longer a hardcoded list.
 *
 * They come from GET /api/alerts/conditions, which merges the host's
 * built-in sources with whatever alerting plugins registered. A list
 * here would recreate exactly the coupling the backend's source
 * registry removes — a plugin's condition would be invisible until
 * someone edited this file.
 *
 * See useAlertConditions.
 */

/** Common cooldown choices, in seconds. 0 means "notify once". */
export const ALERT_COOLDOWN_SECONDS = [0, 300, 900, 1800, 3600, 21600, 86400];

/** Common sustain / window durations, in seconds. */
export const ALERT_DURATION_SECONDS = [0, 60, 300, 600, 900, 1800, 3600, 21600, 86400];
