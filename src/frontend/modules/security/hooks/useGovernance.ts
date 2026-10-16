// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ExplainReason = {
  kind: string;
  id?: string;
  name?: string;
  effect?: string;
};

export type Explanation = {
  permission: string;
  scope: string;
  allowed: boolean;
  policyAllowed: boolean;
  abacEnabled: boolean;
  reasons: ExplainReason[];
};

export type GateResult = {
  scope: string;
  any: boolean;
  allowed: boolean;
  decisions: Explanation[];
};

export type SimulateResult = {
  allowed: boolean;
  route?: string;
  method?: string;
  exempt?: string;
  gates: GateResult[];
};

export type SimulateInput = {
  userId: string;
  environmentId: string;
  stackName: string;
  permission: string;
  method: string;
  path: string;
};

export type ConflictWarning = {
  kind: 'wildcard_grant' | 'category_wildcard' | 'policy_shadowed' | 'policy_tie';
  severity: 'warning' | 'info';
  subjectType: 'role' | 'policy';
  subjectId: string;
  subjectName: string;
  relatedId?: string;
  relatedName?: string;
  permissions: string[];
};

export function useSimulatePolicy() {
  return useMutation({
    mutationFn: (input: SimulateInput) => api.post<SimulateResult>('/governance/simulate', input).then(assertSuccess),
  });
}

export function usePolicyConflicts(enabled = true) {
  return useQuery({
    queryKey: ['governance', 'conflicts'],
    enabled,
    queryFn: () => api.get<ConflictWarning[]>('/governance/conflicts').then(assertSuccess),
  });
}
