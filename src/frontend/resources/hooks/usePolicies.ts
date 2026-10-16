// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// Policy mirrors the ABACPolicy struct exposed by
// core/rbac/policy.go. Keep these fields in sync if you add new
// server-side fields; the admin UI consumes the JSON shape as-is.
export type Policy = {
  id: string;
  name: string;
  description: string;
  effect: 'allow' | 'deny';
  resourceType: string;
  expression: string;
  priority: number;
  enabled: boolean;
  permissions: string[];
  createdBy: string;
  createdAt: string;
  updatedAt: string;
};

export type PolicyInput = {
  name: string;
  description: string;
  effect: 'allow' | 'deny';
  resourceType: string;
  expression: string;
  priority: number;
  enabled: boolean;
  permissions: string[];
};

export type PolicyTestInput = {
  // The expression + attributes to evaluate. The server returns a
  // boolean result + a human-readable reason and any sandbox error.
  resourceType: string;
  expression: string;
  attributes: PolicyTestAttributes;
};

/** Sample attributes for a dry-run, mirrors backend AttributesDTO. */
export type PolicyTestAttributes = {
  user?: {
    id: string;
    username: string;
    email: string;
    roles: string[];
    groups: string[];
    labels: Record<string, string>;
  };
  env?: { id: string; name: string; type: string };
  resource: {
    type: string;
    id: string;
    owner: string;
    image: string;
    state: string;
    labels: Record<string, string>;
  };
  now?: { year: number; month: number; day: number; hour: number; weekday: number };
};

export type PolicyTestResult = {
  matched: boolean;
  reason: string;
  error?: string;
};

const base = '/rbac/policies';

export function usePolicies() {
  return useQuery({
    queryKey: ['abac-policies'],
    queryFn: () => api.get<Policy[]>(base).then((r) => r.data ?? []),
  });
}

export function useCreatePolicy() {
  const qc = useQueryClient();
  const { t } = useTranslation('abac');
  return useMutation({
    mutationFn: (input: PolicyInput) => api.post<Policy>(base, input).then(assertSuccess),
    meta: { success: () => t('toast.created') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['abac-policies'] }),
  });
}

export function useUpdatePolicy() {
  const qc = useQueryClient();
  const { t } = useTranslation('abac');
  return useMutation({
    mutationFn: ({ id, ...input }: PolicyInput & { id: string }) =>
      api.put<Policy>(`${base}/${id}`, input).then(assertSuccess),
    meta: { success: () => t('toast.updated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['abac-policies'] }),
  });
}

export function useDeletePolicy() {
  const qc = useQueryClient();
  const { t } = useTranslation('abac');
  return useMutation({
    mutationFn: (id: string) => api.del(`${base}/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.deleted') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['abac-policies'] }),
  });
}

export function useTogglePolicy() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      api.post(`${base}/${id}/enable`, { enabled }).then(assertSuccess),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['abac-policies'] }),
  });
}

export function useTestPolicy(id: string) {
  return useMutation({
    mutationFn: (attributes: PolicyTestAttributes) =>
      api.post<PolicyTestResult>(`${base}/${id}/test`, { attributes }).then(assertSuccess),
  });
}

// User labels (free-form bag) — used by ABAC authors to inspect
// what they can match against user.labels in policy expressions.
export function useUserLabels(userID: string) {
  return useQuery({
    queryKey: ['abac-user-labels', userID],
    queryFn: () =>
      api.get<{ labels: Record<string, string> }>(`/rbac/users/${userID}/labels`).then((r) => r.data?.labels ?? {}),
    enabled: !!userID,
  });
}
