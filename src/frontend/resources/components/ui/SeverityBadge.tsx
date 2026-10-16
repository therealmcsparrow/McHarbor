// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// SeverityBadge — canonical surface for a 5-level severity scale
// (info / success / warning / error / critical).
//
// Use this in:
//   - alert list rows and alert detail headers
//   - audit log entries
//   - lifecycle events
//   - notification feed items
//   - any toast that carries a severity tag
//
// The 5 tones are deliberately distinguishable in both light and dark
// themes:
//   - info      → sky
//   - success   → emerald
//   - warning   → amber
//   - error     → red (soft)
//   - critical  → red (stronger fill + ring) — louder than error
//
// Implementation note: the badge uses the shared `SeverityIcon`
// primitive so iconography + colour stay in sync, and the label is
// translated through the `severity` namespace in common.json.

import { useTranslation } from 'react-i18next';
import { cn } from '@resources/utils/cn';
import { severityDescriptor, severitySoft, type SeverityLevel } from '@resources/theme/status-palette';
import { SeverityIcon } from './SeverityIcon';

type SeverityBadgeProps = {
  level: SeverityLevel | (string & {});
  /** Optional override for the visible text; default is `t('severity.<level>')`. */
  label?: string;
  className?: string;
};

export function SeverityBadge({ level, label, className }: SeverityBadgeProps) {
  const { t } = useTranslation('common');
  const descriptor = severityDescriptor(level);
  const resolvedLabel = label ?? t(`severity.${descriptor.level}`);

  return (
    <output
      className={cn('inline-flex items-center gap-1.5 px-2 py-0.5 text-xs font-medium', severitySoft(level), className)}
      aria-label={resolvedLabel}
    >
      <SeverityIcon severity={descriptor.level} size="sm" ariaLabel={resolvedLabel} />
      <span>{resolvedLabel}</span>
    </output>
  );
}
