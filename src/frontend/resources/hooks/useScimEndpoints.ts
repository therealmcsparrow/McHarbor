// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// Endpoint mirrors the SCIM Endpoint struct from
// modules/scim/model.go. The server NEVER returns the plaintext
// bearer token — instead the create response includes a one-shot
// bearerToken field that the UI displays exactly once.
export type Endpoint = {
  id: string;
  name: string;
  baseUrl: string;
  authType: string;
  pollSchedule: string;
  enabled: boolean;
  hasToken: boolean;
  username?: string;
  lastSyncAt?: string;
  lastSyncStatus: 'pending' | 'success' | 'failed' | (string & {});
  lastSyncError: string;
  usersCount: number;
  groupsCount: number;
  tokenRotatedAt?: string;
  createdAt: string;
  updatedAt: string;
};

// CreateResponse includes the plaintext token exactly once.
export type CreateEndpointResponse = {
  endpoint: Endpoint;
  bearerToken: string;
};

export type EndpointInput = {
  name: string;
  baseUrl: string;
  authType: string;
  bearerToken?: string;
  username?: string;
  pollSchedule: string;
  enabled: boolean;
};

export type SyncLogEntry = {
  id: string;
  endpointId: string;
  direction: 'pull' | 'push';
  startedAt: string;
  finishedAt?: string;
  status: 'running' | 'success' | 'failed';
  usersCreated: number;
  usersUpdated: number;
  usersDeleted: number;
  groupsCreated: number;
  groupsUpdated: number;
  groupsDeleted: number;
  error: string;
  usersSeen: number;
  usersLinked: number;
  usersUnmatched: number;
  usersDisabled: number;
  usersSkipped: number;
  groupsSeen: number;
  dryRun: boolean;
  trigger: 'schedule' | 'manual' | 'preview' | (string & {});
  notes: string[];
};

export type SyncAction = 'link' | 'unmatched' | 'update' | 'disable' | 'skip';

export type UserChange = {
  action: SyncAction;
  scimId: string;
  userName: string;
  email?: string;
  displayName?: string;
  localUserId?: string;
  localUsername?: string;
  fields?: string[];
  reason?: string;
  link?: boolean;
};

export type SyncSummary = {
  usersSeen: number;
  linked: number;
  unmatched: number;
  updated: number;
  disabled: number;
  skipped: number;
  unchanged: number;
  groupsSeen: number;
  pendingLink: number;
};

export type GroupSummary = {
  scimId: string;
  displayName: string;
  members: number;
  localGroupId?: string;
};

export type SyncPlan = {
  endpointId: string;
  generatedAt: string;
  users: UserChange[];
  groups: GroupSummary[];
  summary: SyncSummary;
  blocked: boolean;
  blockedReason?: string;
};

export type DryRunResult = {
  status: 'success' | 'failed';
  error?: string;
  plan?: SyncPlan;
};

export type RotateTokenResponse = {
  endpoint: Endpoint;
  bearerToken: string;
};

const base = '/scim/endpoints';

export function useEndpoints() {
  return useQuery({
    queryKey: ['scim-endpoints'],
    queryFn: () => api.get<Endpoint[]>(base).then((r) => r.data ?? []),
  });
}

export function useCreateEndpoint() {
  const qc = useQueryClient();
  const { t } = useTranslation('scim');
  return useMutation({
    mutationFn: (input: EndpointInput) => api.post<CreateEndpointResponse>(base, input).then(assertSuccess),
    meta: { success: () => t('toast.created') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['scim-endpoints'] }),
  });
}

export function useUpdateEndpoint() {
  const qc = useQueryClient();
  const { t } = useTranslation('scim');
  return useMutation({
    mutationFn: ({ id, ...input }: EndpointInput & { id: string }) =>
      api.put<Endpoint>(`${base}/${id}`, input).then(assertSuccess),
    meta: { success: () => t('toast.updated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['scim-endpoints'] }),
  });
}

export function useDeleteEndpoint() {
  const qc = useQueryClient();
  const { t } = useTranslation('scim');
  return useMutation({
    mutationFn: (id: string) => api.del(`${base}/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.deleted') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['scim-endpoints'] }),
  });
}

export function useSyncNow() {
  const qc = useQueryClient();
  const { t } = useTranslation('scim');
  return useMutation({
    mutationFn: (id: string) => api.post(`${base}/${id}/sync-now`, {}).then(assertSuccess),
    meta: { success: () => t('toast.syncQueued') },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['scim-endpoints'] });
      void qc.invalidateQueries({ queryKey: ['scim-endpoint-history'] });
    },
  });
}

export function useEndpointHistory(id: string, limit = 50) {
  return useQuery({
    queryKey: ['scim-endpoint-history', id, limit],
    queryFn: () => api.get<SyncLogEntry[]>(`${base}/${id}/history?limit=${limit}`).then((r) => r.data ?? []),
    enabled: !!id,
  });
}
export function useDryRun(id: string, enabled: boolean) {
  return useQuery({
    queryKey: ['scim-dry-run', id],
    queryFn: () => api.post<DryRunResult>(`${base}/${id}/dry-run`, {}).then(assertSuccess),
    enabled: enabled && !!id,
    retry: false,
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
}

export function useRotateToken() {
  const qc = useQueryClient();
  const { t } = useTranslation('scim');
  return useMutation({
    mutationFn: (id: string) => api.post<RotateTokenResponse>(`${base}/${id}/rotate-token`, {}).then(assertSuccess),
    meta: { success: () => t('toast.tokenRotated') },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['scim-endpoints'] }),
  });
}
