// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type StepKind = 'instruction' | 'diagnostic' | 'action';
/** The attach kinds the host owns; a plugin declares any other through its runbook-provider capability. */
export const HOST_ATTACH_KINDS = ['alert_condition', 'container', 'stack', 'environment'] as const;
/** A host attach kind or one a loaded plugin declares; a stored kind whose plugin is gone is an orphan. */
export type AttachKind = string;
/** A selector kind: the host's own ('container', 'environment') or any a loaded plugin declares. */
export type TargetKind = string;
export type ParamType = 'target' | 'enum' | 'int' | 'string' | 'bool';
export type StepState = 'done' | 'skipped' | 'failed';
export type RunOutcome = 'in_progress' | 'completed' | 'abandoned' | 'escalated';
export type RunTrigger = 'manual' | 'alert' | 'schedule';

export type RunbookAutomation = {
  principalId?: string;
  principalName?: string;
  onAlert: boolean;
  cron?: string;
  since?: string;
  nextRunAt?: string;
  lastRunAt?: string;
  lastError?: string;
  updatedBy?: string;
};

export type AutomationInput = {
  principalId: string;
  onAlert: boolean;
  cron: string;
};

export type ServicePrincipal = {
  id: string;
  username: string;
  displayName?: string;
  active: boolean;
};

export type RunbookStep = {
  id: string;
  runbookId: string;
  position: number;
  kind: StepKind;
  title: string;
  body?: string;
  diagnosticKey?: string;
  actionKey?: string;
  actionParams?: Record<string, string>;
  required: boolean;
  requiredPermissions?: string[];
};

export type Runbook = {
  id: string;
  name: string;
  description: string;
  attachKind: AttachKind;
  attachSourceType?: string;
  attachConditionKey?: string;
  attachAlertId?: string;
  attachSubject?: string;
  environmentId?: string;
  enabled: boolean;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
  steps: RunbookStep[];
  automation?: RunbookAutomation;
};

export type RunbookStepResult = {
  id: string;
  runId: string;
  stepId: string;
  stepPosition: number;
  stepTitle: string;
  stepKind: StepKind;
  state: StepState;
  skipReason?: string;
  output?: string;
  finding?: string;
  actedBy: string;
  actedByName: string;
  at: string;
  lastError?: string;
};

export type RunbookRun = {
  id: string;
  runbookId: string;
  runbookName: string;
  environmentId?: string;
  subjectKind?: string;
  subjectId?: string;
  subjectName?: string;
  incidentId?: string;
  alertEventId?: string;
  startedBy: string;
  startedByName: string;
  startedAt: string;
  finishedAt?: string;
  outcome: RunOutcome;
  note?: string;
  results: RunbookStepResult[];
  trigger?: RunTrigger;
};

export type ResolvedRunbooks = {
  runbooks: Runbook[];
  openRuns: RunbookRun[];
};

export * from './runbookCatalog';

export type RunbookTarget = {
  environmentId?: string;
  sourceType?: string;
  conditionKey?: string;
  alertId?: string;
  alertEventId?: string;
  container?: string;
  stack?: string;
  kind?: AttachKind;
  id?: string;
};

export type StepInput = {
  kind: StepKind;
  title: string;
  body: string;
  diagnosticKey: string;
  actionKey: string;
  actionParams: Record<string, string>;
  required: boolean;
};

export type RunbookInput = {
  name: string;
  description: string;
  attachKind: AttachKind;
  attachSourceType: string;
  attachConditionKey: string;
  attachAlertId: string;
  attachSubject: string;
  environmentId: string;
  enabled: boolean;
  steps: StepInput[];
};

export function stepResultFor(run: RunbookRun | null, step: RunbookStep): RunbookStepResult | null {
  if (!run) return null;
  const matches = run.results.filter((result) => result.stepId === step.id);
  return matches.length === 0 ? null : (matches[matches.length - 1] ?? null);
}

export function firstOpenStep(run: RunbookRun | null, steps: RunbookStep[]): number {
  if (!run) return 0;
  for (let i = 0; i < steps.length; i += 1) {
    const step = steps[i];
    if (!step) continue;
    const result = stepResultFor(run, step);
    if (!result || result.state === 'failed') return i;
  }
  return Math.max(steps.length - 1, 0);
}

export function stoppedAtPosition(run: RunbookRun): number {
  return run.results.reduce((highest, result) => Math.max(highest, result.stepPosition), 0);
}
