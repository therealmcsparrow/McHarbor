// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import type { Runbook, RunbookInput, StepInput } from '@resources/types/runbooks';

export const blankStep: StepInput = {
  kind: 'instruction',
  title: '',
  body: '',
  diagnosticKey: '',
  actionKey: '',
  actionParams: {},
  required: true,
};

export function blankInput(envId: string): RunbookInput {
  return {
    name: '',
    description: '',
    attachKind: 'alert_condition',
    attachSourceType: 'container',
    attachConditionKey: '',
    attachAlertId: '',
    attachSubject: '',
    environmentId: envId,
    enabled: true,
    steps: [{ ...blankStep }],
  };
}

export function inputFromRunbook(loaded: Runbook): RunbookInput {
  return {
    name: loaded.name,
    description: loaded.description,
    attachKind: loaded.attachKind,
    attachSourceType: loaded.attachSourceType ?? '',
    attachConditionKey: loaded.attachConditionKey ?? '',
    attachAlertId: loaded.attachAlertId ?? '',
    attachSubject: loaded.attachSubject ?? '',
    environmentId: loaded.environmentId ?? '',
    enabled: loaded.enabled,
    steps: loaded.steps.map((step) => ({
      kind: step.kind,
      title: step.title,
      body: step.body ?? '',
      diagnosticKey: step.diagnosticKey ?? '',
      actionKey: step.actionKey ?? '',
      actionParams: step.actionParams ?? {},
      required: step.required,
    })),
  };
}

let nextKey = 0;
function newKey(): string {
  nextKey += 1;
  return `step-${nextKey}`;
}

export function useRunbookDraft(loaded: Runbook | undefined, envId: string) {
  const [draft, setDraft] = useState<RunbookInput>(() => (loaded ? inputFromRunbook(loaded) : blankInput(envId)));
  const [keys, setKeys] = useState<string[]>(() => draft.steps.map(newKey));
  const [baseline] = useState(() => JSON.stringify(draft));

  const dirty = useMemo(() => JSON.stringify(draft) !== baseline, [draft, baseline]);

  const patch = useCallback((next: Partial<RunbookInput>) => setDraft((current) => ({ ...current, ...next })), []);

  const updateStep = useCallback((index: number, next: StepInput) => {
    setDraft((current) => ({ ...current, steps: current.steps.map((step, i) => (i === index ? next : step)) }));
  }, []);

  const addStep = useCallback(() => {
    setDraft((current) => ({ ...current, steps: [...current.steps, { ...blankStep }] }));
    setKeys((current) => [...current, newKey()]);
  }, []);

  const removeStep = useCallback((index: number) => {
    setDraft((current) => ({ ...current, steps: current.steps.filter((_, i) => i !== index) }));
    setKeys((current) => current.filter((_, i) => i !== index));
  }, []);

  const moveStep = useCallback((index: number, delta: number) => {
    const swap = <T>(items: T[]): T[] => {
      const target = index + delta;
      const a = items[index];
      const b = items[target];
      if (a === undefined || b === undefined) return items;
      const next = [...items];
      next[index] = b;
      next[target] = a;
      return next;
    };
    setDraft((current) => ({ ...current, steps: swap(current.steps) }));
    setKeys(swap);
  }, []);

  return { draft, keys, dirty, patch, updateStep, addStep, removeStep, moveStep };
}
