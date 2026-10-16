// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Shared formatting for the lifecycle log, used by both the table columns and
// the details dialog. Previously private to the LifecycleRow component.

import { IconBox, IconDeviceFloppy, IconNetwork, IconPhoto, IconStack2, type TablerIcon } from '@tabler/icons-react';
import type { HostLifecycleSubjectType, LifecycleEvent, LifecycleSeverity } from '../hooks/useLifecycle';

/** Host subjects only; a plugin-declared subject takes the caller's fallback icon. */
export const SUBJECT_ICON: Readonly<Record<string, TablerIcon | undefined>> = {
  container: IconBox,
  image: IconPhoto,
  volume: IconDeviceFloppy,
  network: IconNetwork,
  stack: IconStack2,
} satisfies Record<HostLifecycleSubjectType, TablerIcon>;

export const SEVERITY_LABEL: Record<LifecycleSeverity, string> = {
  success: 'lifecycle.severity.success',
  info: 'lifecycle.severity.info',
  warning: 'lifecycle.severity.warning',
  error: 'lifecycle.severity.error',
};

/** The structural subset the lifecycle UI needs from the environment store. */
export type EnvironmentLite = { id: string; name: string };

export function parseMetadata(raw: string | undefined): Record<string, string> | null {
  if (!raw) return null;
  try {
    const obj = JSON.parse(raw);
    if (obj && typeof obj === 'object') return obj as Record<string, string>;
  } catch {
    // A malformed metadata blob is not worth failing a row over.
  }
  return null;
}

/** `pluginPages` maps a plugin-owned subject type to its page (useLifecycleSubjectPages). */
export function buildSubjectPath(subject: LifecycleEvent, pluginPages: Record<string, string>): string {
  switch (subject.subjectType) {
    case 'container':
      return `/containers/${subject.subjectId}`;
    case 'image':
      return `/images/${subject.subjectId}`;
    case 'volume':
      return `/volumes/${subject.subjectId}`;
    case 'network':
      return `/networks/${subject.subjectId}`;
    default:
      return pluginPages[subject.subjectType] ?? '#';
  }
}

export function humanizeAction(action: string, t: (key: string) => string): string {
  const translated = t(`lifecycle.action.${action}`);
  return translated === `lifecycle.action.${action}` ? action : translated;
}
