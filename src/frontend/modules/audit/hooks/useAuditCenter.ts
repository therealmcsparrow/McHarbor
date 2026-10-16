// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { DateRange } from '@resources/utils/date-range';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';

export const AUDIT_SEVERITIES = ['info', 'success', 'warning', 'error'] as const;
export type AuditSeverity = (typeof AUDIT_SEVERITIES)[number];

export interface AuditDenial {
  id: string;
  timestamp: string;
  kind: 'rbac' | 'abac';
  userId: string;
  username: string;
  permission: string;
  route: string;
  environmentId?: string | null;
  environmentName?: string | null;
  policyId?: string;
  resourceType?: string;
  resourceId?: string;
  ipAddress?: string | null;
}

export interface AITrailStep {
  kind: string;
  auditId?: string;
  timestamp: string;
  actor?: string;
  details?: string;
}

export interface AITrailGroup {
  actionId: string;
  tool: string;
  conversationId?: string;
  userId?: string;
  username?: string;
  environmentId?: string | null;
  status: string;
  startedAt: string;
  lastAt: string;
  arguments?: string;
  plan?: string;
  result?: string;
  error?: string;
  truncated: boolean;
  recordMissing: boolean;
  steps: AITrailStep[];
}

export interface AuditRetentionView {
  defaultDays: number;
  severities: AuditSeverity[];
  overrides: Record<AuditSeverity, number | null>;
  effective: Record<AuditSeverity, number>;
  maxDays: number;
}

export interface AuditRestoreAnchor {
  id: string;
  timestamp: string;
  actor: string;
  details: string;
}

export interface AuditVerifyResult {
  verified: boolean;
  complete: boolean;
  keyConfigured: boolean;
  totalRows: number;
  checkedRows: number;
  hashedRows: number;
  unsealedRows: number;
  truncated: boolean;
  firstBrokenId?: string;
  reason?: string;
  checkedAt: string;
  anchors: AuditRestoreAnchor[];
  prunedGaps: number;
  prunedRows: number;
  tombstones: number;
  invalidTombstones: number;
}

export interface AuditCenterQuery {
  page: number;
  perPage: number;
  actor?: string;
  range?: DateRange;
}

export function centerParams({ page, perPage, actor = '', range }: AuditCenterQuery): Record<string, string> {
  const params: Record<string, string> = { page: String(page), per_page: String(perPage) };
  if (actor.trim()) params.actor = actor.trim();
  if (range?.from) params.from = range.from.toISOString();
  if (range?.to) params.to = range.to.toISOString();
  return params;
}

export function useAuditDenials(query: AuditCenterQuery & { permission?: string; environment?: string }) {
  const params = centerParams(query);
  if (query.permission?.trim()) params.permission = query.permission.trim();
  if (query.environment) params.environment = query.environment;
  return useQuery({
    queryKey: ['audit-denials', params],
    queryFn: async () => assertSuccess(await api.get<PaginatedData<AuditDenial>>('/audit/denials', params)),
    placeholderData: (previous) => previous,
    staleTime: 15_000,
  });
}

export function useAIActionTrail(query: AuditCenterQuery) {
  const params = centerParams(query);
  return useQuery({
    queryKey: ['audit-ai-actions', params],
    queryFn: async () => assertSuccess(await api.get<PaginatedData<AITrailGroup>>('/audit/ai-actions', params)),
    placeholderData: (previous) => previous,
    staleTime: 15_000,
  });
}

export function useAIToolEvents(query: AuditCenterQuery) {
  const params = centerParams(query);
  return useQuery({
    queryKey: ['audit-ai-tools', params],
    queryFn: async () => assertSuccess(await api.get<PaginatedData<AuditEntry>>('/audit/ai-tools', params)),
    placeholderData: (previous) => previous,
    staleTime: 15_000,
  });
}

export function useAuditRetention() {
  return useQuery({
    queryKey: ['audit-retention'],
    queryFn: async () => assertSuccess(await api.get<AuditRetentionView>('/audit/retention')),
    staleTime: 30_000,
  });
}

export function useUpdateAuditRetention() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (overrides: Partial<Record<AuditSeverity, number | null>>) =>
      assertSuccess(await api.put<AuditRetentionView>('/audit/retention', { overrides })),
    onSuccess: (view) => {
      client.setQueryData(['audit-retention'], view);
    },
  });
}

export function useVerifyAuditChain() {
  return useMutation({
    mutationFn: async () => assertSuccess(await api.get<AuditVerifyResult>('/audit/verify')),
  });
}
