// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Filter option lists for the lifecycle log, shared by the page (which needs
// the allowed-value arrays for URL state and the labels for chips) and the
// filter controls component.

import type { HostLifecycleSubjectType, LifecycleSeverity } from '../hooks/useLifecycle';

/** The host's own subjects; plugin subjects are added by useLifecycleSubjectTypes. */
export const SUBJECT_TYPES: { value: HostLifecycleSubjectType | ''; labelKey: string }[] = [
  { value: '', labelKey: 'lifecycle.subjectType.all' },
  { value: 'container', labelKey: 'lifecycle.subjectType.container' },
  { value: 'image', labelKey: 'lifecycle.subjectType.image' },
  { value: 'volume', labelKey: 'lifecycle.subjectType.volume' },
  { value: 'network', labelKey: 'lifecycle.subjectType.network' },
  { value: 'stack', labelKey: 'lifecycle.subjectType.stack' },
];

export const SEVERITIES: { value: LifecycleSeverity | ''; labelKey: string }[] = [
  { value: '', labelKey: 'lifecycle.severity.all' },
  { value: 'success', labelKey: 'lifecycle.severity.success' },
  { value: 'info', labelKey: 'lifecycle.severity.info' },
  { value: 'warning', labelKey: 'lifecycle.severity.warning' },
  { value: 'error', labelKey: 'lifecycle.severity.error' },
];

export const severityValues = SEVERITIES.map((item) => item.value);
