// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { DemoConfig, DemoConfigField, DemoLimits, DemoSizes, DemoStatus } from '@resources/types/demo';

export type DemoConfigGroup = 'estate' | 'compute' | 'kubernetes' | 'swarm' | 'history';

export const DEMO_CONFIG_GROUPS: readonly { key: DemoConfigGroup; fields: readonly DemoConfigField[] }[] = [
  { key: 'estate', fields: ['environments', 'containers'] },
  { key: 'compute', fields: ['virtualMachines', 'systemContainers'] },
  { key: 'kubernetes', fields: ['kubernetesClusters', 'kubernetesDeployments'] },
  { key: 'swarm', fields: ['swarmClusters', 'swarmNodes', 'swarmServices'] },
  { key: 'history', fields: ['historyDays'] },
];

export const DEMO_CONFIG_FIELDS: readonly DemoConfigField[] = DEMO_CONFIG_GROUPS.flatMap((group) => group.fields);

export type DemoConfigDraft = Record<DemoConfigField, string>;

export type DemoFieldProblem = { kind: 'range' | 'swarmNodes'; min: number; max: number };

export type DemoConfigProblems = Partial<Record<DemoConfigField, DemoFieldProblem>>;

export type DemoConfigCheck = { config?: DemoConfig; problems: DemoConfigProblems };

export function isDemoConfigField(value: unknown): value is DemoConfigField {
  return DEMO_CONFIG_FIELDS.some((field) => field === value);
}

export function demoSizesOf(currentStatus: DemoStatus): DemoSizes {
  return {
    initial: currentStatus.config ?? currentStatus.defaults,
    defaults: currentStatus.defaults,
    limits: currentStatus.limits,
  };
}

export function draftOf(config: DemoConfig): DemoConfigDraft {
  const draft: Partial<DemoConfigDraft> = {};
  for (const field of DEMO_CONFIG_FIELDS) draft[field] = String(config[field]);
  return draft as DemoConfigDraft;
}

export function sameDemoConfig(a: DemoConfig, b: DemoConfig): boolean {
  return DEMO_CONFIG_FIELDS.every((field) => a[field] === b[field]);
}

function wholeNumber(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return undefined;
  return Number(trimmed);
}

export function checkDemoConfig(draft: DemoConfigDraft, limits: DemoLimits): DemoConfigCheck {
  const problems: DemoConfigProblems = {};
  const values: Partial<DemoConfig> = {};
  for (const field of DEMO_CONFIG_FIELDS) {
    const { min, max } = limits[field];
    const value = wholeNumber(draft[field]);
    if (value === undefined || value < min || value > max) {
      problems[field] = { kind: 'range', min, max };
      continue;
    }
    values[field] = value;
  }
  const clusters = values.swarmClusters;
  const nodes = values.swarmNodes;
  if (clusters !== undefined && nodes !== undefined && clusters > 0 && nodes < clusters) {
    problems.swarmNodes = { kind: 'swarmNodes', min: clusters, max: limits.swarmNodes.max };
  }
  if (Object.keys(problems).length > 0) return { problems };
  return { config: values as DemoConfig, problems };
}
