// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';

/**
 * Just-in-time privilege elevation: request a permission for a bounded window,
 * have an approver decide, and hold it until it lapses.
 *
 * The API has existed for a while; until 2026-09-11 nothing called it *and*
 * nothing enforced it — `core/rbac` never read `elevation_requests`, so an
 * approval granted nothing. The authorization hook landed with these hooks, so
 * an approval now actually widens the requester's effective permissions until
 * `approvedUntil`.
 */
export type ElevationRequest = {
  id: string;
  requesterId: string;
  permissionKey: string;
  permissionLabel?: string;
  environmentId?: string;
  resourceType?: string;
  resourceId?: string;
  justification: string;
  requestedMinutes: number;
  status: 'pending' | 'approved' | 'denied' | 'expired';
  approverId?: string;
  approverNote?: string;
  approvedUntil?: string;
  decidedAt?: string;
  createdAt: string;
};

export type CreateElevationInput = {
  permissionKey: string;
  environmentId?: string;
  justification: string;
  requestedMinutes: number;
};

// The list handlers return `{"requests": [...]}`, not a bare array
// (handler.go: response.OK(w, map[string]any{"requests": items})). Typing the
// response as ElevationRequest[] is what produced "e.map is not a function".
export type ElevatableEntry = {
  key: string;
  category: string;
  action: string;
  description: string;
  origin: string;
  elevatable: boolean;
  source: 'admin' | 'default';
};

const MINE = ['elevations', 'mine'] as const;
const ELEVATABLE = ['elevations', 'elevatable'] as const;
const PENDING = ['elevations', 'pending'] as const;

/** Anyone signed in may see their own requests: the route carries no key. */
export function useMyElevations() {
  return useQuery({
    queryKey: MINE,
    queryFn: () => api.get<{ requests: ElevationRequest[] }>('/elevations/mine').then((r) => r.data?.requests ?? []),
  });
}

/**
 * The approver inbox. `users.manage` gates the route, so the query is disabled
 * without it rather than firing a 403 for every non-approver who opens the tab.
 */
export function usePendingElevations() {
  const allowed = usePermission(PERM.USERS_MANAGE);
  return useQuery({
    enabled: allowed,
    queryKey: PENDING,
    queryFn: () => api.get<{ requests: ElevationRequest[] }>('/elevations/pending').then((r) => r.data?.requests ?? []),
    refetchInterval: 60_000,
  });
}

export function useCreateElevation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (input: CreateElevationInput) => api.post<ElevationRequest>('/elevations', input).then(assertSuccess),
    meta: { success: t('elevations.toast.requested') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: MINE });
      void queryClient.invalidateQueries({ queryKey: PENDING });
    },
  });
}

export function useDecideElevation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ id, approve, note }: { id: string; approve: boolean; note?: string }) =>
      api
        .post<ElevationRequest>(`/elevations/${encodeURIComponent(id)}/decide`, {
          approve,
          note: note ?? '',
        })
        .then(assertSuccess),
    meta: { success: t('elevations.toast.decided') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: PENDING });
      void queryClient.invalidateQueries({ queryKey: MINE });
      // The decision changes what the requester may do, so anything cached
      // from their permission set is stale the moment this returns.
      void queryClient.invalidateQueries({ queryKey: ['auth', 'permissions'] });
    },
  });
}

export function useWithdrawElevation() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (id: string) => api.del(`/elevations/${encodeURIComponent(id)}`).then(assertSuccess),
    meta: { success: t('elevations.toast.withdrawn') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: MINE });
      void queryClient.invalidateQueries({ queryKey: PENDING });
    },
  });
}

export function useElevatable() {
  return useQuery({
    queryKey: ELEVATABLE,
    queryFn: () => api.get<ElevatableEntry[]>('/elevations/elevatable').then((r) => r.data ?? []),
  });
}

export function useSetElevatable() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (input: { key: string; elevatable: boolean }) =>
      api.put('/elevations/elevatable', input).then(assertSuccess),
    meta: { success: t('elevations.toast.policySaved') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ELEVATABLE });
      void queryClient.invalidateQueries({ queryKey: ['auth', 'permissions'] });
    },
  });
}
