// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { PrerequisiteReport } from './useEnvironmentActions';

export type PrereqGate = {
  /** Required items the operator has neither fixed nor ticked for install. */
  blocking: string[];
  /** Ticked installs cannot run: no escalation route, or su without a root password. */
  noEscalation: boolean;
  ready: boolean;
};

/**
 * An unprobed host never blocks: probing is optional and the deploy reports its
 * own failure. Once a report exists, the deploy method's required items must be
 * present or ticked for install, and a ticked install needs a way to escalate.
 */
export function evaluatePrereqGate(
  report: PrerequisiteReport | undefined,
  selected: string[],
  rootPassword: string,
): PrereqGate {
  if (!report) return { blocking: [], noEscalation: false, ready: true };
  const blocking = report.items
    .filter((item) => item.required && !item.present && !selected.includes(item.key))
    .map((item) => item.key);
  // su-only hosts report `unavailable` until a root password is supplied; the
  // install re-detects with it, so typing one clears the block before a re-probe.
  const suRecoverable = Boolean(report.needsRootPassword) && rootPassword !== '';
  const noEscalation = selected.length > 0 && report.escalation === 'unavailable' && !suRecoverable;
  return { blocking, noEscalation, ready: blocking.length === 0 && !noEscalation };
}
