// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ComplianceFact, ComplianceStatus } from './useCompliance';

export type PackCoverage = 'mechanical' | 'evidenced' | 'notCovered';

export type PackEvidenceSource = {
  key: string;
  kind: 'dockerApi' | 'route' | 'table' | 'setting' | 'absent';
  ref: string;
  notes?: string;
};

export type PackControl = {
  id: string;
  group: string;
  title: string;
  requirement: string;
  coverage: PackCoverage;
  scope: 'daemon' | 'container' | 'none';
  evidence: string[];
};

export type PackFramework = {
  key: string;
  name: string;
  description: string;
  controls: PackControl[];
  evidence: PackEvidenceSource[];
  runnable: boolean;
};

export type PackRunSummary = Record<ComplianceStatus, number>;

export type PackRun = {
  id: string;
  framework: string;
  environmentId: string;
  trigger: 'manual' | 'scheduled';
  startedBy: string;
  startedAt: string;
  finishedAt?: string;
  status: 'running' | 'completed' | 'failed';
  summary: PackRunSummary;
  lastError?: string;
};

export type PackResult = {
  id: string;
  status: ComplianceStatus;
  facts: ComplianceFact[];
  reasons: string[];
};

export type PackRunDetail = {
  run: PackRun;
  results: PackResult[];
};

export type PackSummary = {
  framework: PackFramework;
  latestRun?: PackRun;
};

export function useCompliancePacks(enabled: boolean) {
  return useQuery({
    queryKey: ['compliance', 'packs'],
    enabled,
    queryFn: () => api.get<PackSummary[]>('/compliance/packs').then(assertSuccess),
    staleTime: 30_000,
  });
}

export function useCompliancePackRun(runId: string | undefined) {
  return useQuery({
    queryKey: ['compliance', 'packs', 'run', runId],
    enabled: Boolean(runId),
    queryFn: () => api.get<PackRunDetail>(`/compliance/packs/runs/${runId}`).then(assertSuccess),
    staleTime: 30_000,
  });
}

function runPath(framework: string, environmentId?: string): string {
  const base = `/compliance/packs/${encodeURIComponent(framework)}/run`;
  return environmentId ? `${base}?env=${encodeURIComponent(environmentId)}` : base;
}

export function useStartCompliancePackRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { framework: string; environmentId?: string }) =>
      api.post<{ runId: string }>(runPath(input.framework, input.environmentId)).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compliance', 'packs'] });
    },
  });
}

export function notAssessedCount(summary: PackRunSummary | undefined): number {
  return summary ? summary.unknown : 0;
}

export function groupControls(controls: PackControl[]): [string, PackControl[]][] {
  const groups: [string, PackControl[]][] = [];
  for (const control of controls) {
    const existing = groups.find(([itemName]) => itemName === control.group);
    if (existing) existing[1].push(control);
    else groups.push([control.group, [control]]);
  }
  return groups;
}
