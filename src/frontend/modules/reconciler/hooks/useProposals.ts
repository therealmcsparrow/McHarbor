// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { DriftReport } from './useReconciler';
import { refusalOf } from './refusal';

export type StepAction = 'start' | 'stop' | 'remove' | 'recreate' | 'stack_up' | 'manual' | 'plugin';

export type ManualReason =
  | 'missing'
  | 'compose_managed'
  | 'ports'
  | 'mounts'
  | 'networks'
  | 'protected'
  | 'stack_remove'
  | 'stack_stop'
  | 'git_compose';

export type PlanStep = {
  action: StepAction;
  target: string;
  fields?: string[];
  reason?: ManualReason;
  pluginAction?: string;
  detail?: string;
  disruptive?: boolean;
};

export type StepResult = {
  action: StepAction;
  target: string;
  status: 'done' | 'failed' | 'skipped' | 'manual';
  error?: string;
};

export type ProposalStatus = 'pending' | 'applying' | 'applied' | 'failed' | 'rejected' | 'superseded';

export type Proposal = {
  id: string;
  desiredStateId: string;
  desiredName: string;
  kind: string;
  envId: string;
  origin: 'propose' | 'enforce';
  status: ProposalStatus;
  fingerprint: string;
  report?: DriftReport;
  steps: PlanStep[];
  results: StepResult[];
  error?: string;
  note?: string;
  resolvedBy?: string;
  resolvedAt?: string;
  createdAt: string;
};

export type ProposalOutcome = { proposal: Proposal; stale: boolean };

const KEY = ['reconciler-proposals'] as const;

export function useProposals(statusValue: ProposalStatus | '') {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: [...KEY, statusValue, envId],
    queryFn: () =>
      api
        .get<PaginatedData<Proposal>>('/reconciler/proposals', {
          per_page: '200',
          ...(statusValue ? { status: statusValue } : {}),
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data?.items ?? []),
    refetchInterval: 15_000,
  });
}

function useInvalidate() {
  const client = useQueryClient();
  return () => {
    void client.invalidateQueries({ queryKey: KEY });
    void client.invalidateQueries({ queryKey: ['reconciler'] });
  };
}

export function useApproveProposal() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<ProposalOutcome>(`/reconciler/proposals/${encodeURIComponent(id)}/approve`).then(refusalOf),
    onSuccess: invalidate,
  });
}

export function useRejectProposal() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, note }: { id: string; note: string }) =>
      api.post<Proposal>(`/reconciler/proposals/${encodeURIComponent(id)}/reject`, { note }).then(refusalOf),
    onSuccess: invalidate,
  });
}

export function usePropose() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (desiredStateId: string) =>
      api.post<Proposal>(`/reconciler/${encodeURIComponent(desiredStateId)}/propose`).then(refusalOf),
    onSuccess: invalidate,
  });
}
