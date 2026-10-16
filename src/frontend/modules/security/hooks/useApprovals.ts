// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ApprovalStatus = 'pending' | 'approved' | 'denied' | 'expired' | 'cancelled' | 'executing' | 'executed';

export type ApprovalDecision = {
  id: string;
  approverId: string;
  approverName: string;
  decision: 'approve' | 'deny';
  note: string;
  createdAt: string;
};

export type ApprovalRequest = {
  id: string;
  ruleId: string;
  category: string;
  requesterId: string;
  requesterName: string;
  environmentId: string | null;
  method: string;
  path: string;
  routePattern: string;
  resourceName: string;
  bodyRedacted: string;
  status: ApprovalStatus;
  requiredApprovers: number;
  approvalsCount: number;
  approverPermission: string;
  expiresAt: string;
  decidedAt: string | null;
  executedAt: string | null;
  resultStatus: number | null;
  resultCode: string;
  createdAt: string;
  decisions: ApprovalDecision[];
};

export type ApprovalRule = {
  id: string;
  category: string;
  environmentId: string | null;
  enabled: boolean;
  requiredApprovers: number;
  approverPermission: string;
  expiryMinutes: number;
  description: string;
  createdAt: string;
};

export type ApprovalRuleInput = {
  category: string;
  environmentId: string | null;
  enabled: boolean;
  requiredApprovers: number;
  approverPermission: string;
  expiryMinutes: number;
  description: string;
};

export type ApprovalCategory = {
  key: string;
  routes: { method: string; pattern: string }[];
  permissions: string[];
};

export type ExecuteResult = {
  approval: ApprovalRequest;
  resultStatus: number;
};

export type ApprovalScope = 'all' | 'mine';

const KEY = ['approvals'] as const;

export function useApprovalRequests(scope: ApprovalScope, statusValue: string, enabled = true) {
  return useQuery({
    queryKey: [...KEY, 'requests', scope, statusValue],
    enabled,
    queryFn: () =>
      api
        .get<PaginatedData<ApprovalRequest>>(scope === 'mine' ? '/approvals/mine' : '/approvals', {
          ...(statusValue ? { status: statusValue } : {}),
          per_page: '100',
        })
        .then(assertSuccess)
        .then((page) => page.items ?? []),
  });
}

export function useApprovalRules(enabled = true) {
  return useQuery({
    queryKey: [...KEY, 'rules'],
    enabled,
    queryFn: () => api.get<ApprovalRule[]>('/approvals/rules').then(assertSuccess),
  });
}

export function useApprovalCategories(enabled = true) {
  return useQuery({
    queryKey: [...KEY, 'categories'],
    enabled,
    staleTime: 300_000,
    queryFn: () => api.get<ApprovalCategory[]>('/approvals/categories').then(assertSuccess),
  });
}

function useInvalidate() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: KEY });
}

export function useSaveApprovalRule() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, input }: { id?: string; input: ApprovalRuleInput }) =>
      (id
        ? api.put<ApprovalRule>(`/approvals/rules/${id}`, input)
        : api.post<ApprovalRule>('/approvals/rules', input)
      ).then(assertSuccess),
    meta: { success: () => t('governance.rules.saved') },
    onSuccess: invalidate,
  });
}

export function useDeleteApprovalRule() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => api.del(`/approvals/rules/${id}`).then(assertSuccess),
    meta: { success: () => t('governance.rules.deleted') },
    onSuccess: invalidate,
  });
}

export function useDecideApproval() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ id, approve, note }: { id: string; approve: boolean; note: string }) =>
      api.post<ApprovalRequest>(`/approvals/${id}/decide`, { approve, note }).then(assertSuccess),
    meta: {
      success: (_data: unknown, vars: unknown) =>
        (vars as { approve: boolean }).approve ? t('governance.approvals.approved') : t('governance.approvals.denied'),
    },
    onSuccess: invalidate,
  });
}

export function useExecuteApproval() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => api.post<ExecuteResult>(`/approvals/${id}/execute`).then(assertSuccess),
    meta: { success: () => t('governance.approvals.executed') },
    onSuccess: invalidate,
  });
}

export function useCancelApproval() {
  const { t } = useTranslation('security');
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => api.post<ApprovalRequest>(`/approvals/${id}/cancel`).then(assertSuccess),
    meta: { success: () => t('governance.approvals.cancelled') },
    onSuccess: invalidate,
  });
}
