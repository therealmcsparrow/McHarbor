// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconClock,
  IconPlayerPlay,
  IconBolt,
  IconAlertTriangle,
  IconShieldCheck,
  IconHeartbeat,
  IconUrgent,
  IconGitCompare,
  IconArchive,
  IconArrowUpCircle,
} from '@tabler/icons-react';
import type { WidgetDefinitionMeta, WidgetTypeId } from '@resources/widgets/catalog-types';

export const OPERATIONS_WIDGET_META: Partial<Record<WidgetTypeId, WidgetDefinitionMeta>> = {
  uptime: {
    id: 'uptime',
    labelKey: 'widgets.uptime.label',
    descriptionKey: 'widgets.uptime.description',
    category: 'host',
    icon: IconClock,
    defaultSize: { w: 3, h: 1 },
    minSize: { w: 2, h: 1 },
  },
  'workflow-runs': {
    id: 'workflow-runs',
    labelKey: 'widgets.workflowRuns.label',
    descriptionKey: 'widgets.workflowRuns.description',
    category: 'operations',
    icon: IconPlayerPlay,
    defaultSize: { w: 4, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'quick-actions': {
    id: 'quick-actions',
    labelKey: 'widgets.quickActions.label',
    descriptionKey: 'widgets.quickActions.description',
    category: 'operations',
    icon: IconBolt,
    defaultSize: { w: 3, h: 2 },
    minSize: { w: 2, h: 2 },
  },
  'alert-summary': {
    id: 'alert-summary',
    labelKey: 'widgets.alertSummary.label',
    descriptionKey: 'widgets.alertSummary.description',
    category: 'monitoring',
    icon: IconAlertTriangle,
    defaultSize: { w: 4, h: 2 },
    minSize: { w: 3, h: 2 },
  },
  'vulnerability-summary': {
    id: 'vulnerability-summary',
    labelKey: 'widgets.vulnerabilitySummary.label',
    descriptionKey: 'widgets.vulnerabilitySummary.description',
    category: 'monitoring',
    icon: IconShieldCheck,
    defaultSize: { w: 4, h: 2 },
    minSize: { w: 3, h: 2 },
  },

  // ── Verdict widgets ─────────────────────────────────────────────────
  // The catalogue had 33 types and not one of them answered "is anything
  // wrong?" — every one returned a reading rather than a judgement. These
  // four are ordinary catalogue entries: removable, resizable and
  // reorderable exactly like the rest, with co-located i18n.
  'fleet-health': {
    id: 'fleet-health',
    labelKey: 'widgets.fleetHealth.label',
    descriptionKey: 'widgets.fleetHealth.description',
    category: 'monitoring',
    icon: IconHeartbeat,
    // Verdict + facts + bar + legend + the two attention cards need three
    // rows; at h:2 the cards fall below the fold.
    defaultSize: { w: 12, h: 3 },
    minSize: { w: 6, h: 3 },
  },
  'needs-attention': {
    id: 'needs-attention',
    labelKey: 'widgets.needsAttention.label',
    descriptionKey: 'widgets.needsAttention.description',
    category: 'monitoring',
    icon: IconUrgent,
    defaultSize: { w: 6, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'drift-status': {
    id: 'drift-status',
    labelKey: 'widgets.driftStatus.label',
    descriptionKey: 'widgets.driftStatus.description',
    category: 'monitoring',
    icon: IconGitCompare,
    defaultSize: { w: 6, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'instance-backup-status': {
    id: 'instance-backup-status',
    labelKey: 'widgets.instanceBackupStatus.label',
    descriptionKey: 'widgets.instanceBackupStatus.description',
    category: 'monitoring',
    icon: IconArchive,
    defaultSize: { w: 6, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'update-queue': {
    id: 'update-queue',
    labelKey: 'widgets.updateQueue.label',
    descriptionKey: 'widgets.updateQueue.description',
    category: 'operations',
    icon: IconArrowUpCircle,
    defaultSize: { w: 6, h: 3 },
    minSize: { w: 3, h: 2 },
  },
  'snapshot-status': {
    id: 'snapshot-status',
    labelKey: 'widgets.snapshotStatus.label',
    descriptionKey: 'widgets.snapshotStatus.description',
    category: 'operations',
    icon: IconArchive,
    defaultSize: { w: 6, h: 3 },
    minSize: { w: 3, h: 2 },
  },
};
