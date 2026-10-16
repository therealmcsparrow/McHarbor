// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Session listing and revocation, shared by the security module (an admin
// managing any user's sessions) and the profile module (a user managing their
// own). It lives in @resources because both modules need it and a module may
// not import another module - see .claude/rules/react-components.md.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type SessionView = {
  id: string;
  handle: string;
  ipAddress: string;
  userAgent: string;
  createdAt: string;
  expiresAt: string;
  revokedAt?: string;
  revokedBy?: string;
  currentSession: boolean;
};

/**
 * useUserSessions lists the sessions for a single user. Returns the
 * current session flagged via currentSession=true so the UI can render
 * a "this device" badge.
 *
 * Pass `null` to fetch the current user's own sessions (self-service
 * case). Pass a user id to fetch any user's sessions (admin case).
 */
export function useUserSessions(userId: string | null | undefined) {
  return useQuery({
    queryKey: userId ? ['user-sessions', userId] : ['my-sessions'],
    queryFn: () => {
      const path = userId ? `/users/${userId}/sessions` : '/auth/sessions';
      return api.get<SessionView[]>(path).then((r) => r.data ?? []);
    },
    enabled: userId !== undefined,
    refetchInterval: 30_000,
  });
}

export function useRevokeUserSession() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ userId, sessionId }: { userId: string; sessionId: string }) => {
      // Admin path uses /users/{id}/sessions/{sessionId}; self path
      // uses /auth/sessions/{sessionId}. Detect via whether the
      // caller is asking for a specific user id (admin) or the
      // generic self path (no userId supplied — see useUserSessions).
      // The shape below handles both via the boolean flag.
      const path = `/users/${userId}/sessions/${sessionId}`;
      return api.del(path).then(assertSuccess);
    },
    meta: { success: () => t('toast.sessionRevoked') },
    onSuccess: (_, vars) => {
      void queryClient.invalidateQueries({ queryKey: ['user-sessions', vars.userId] });
      void queryClient.invalidateQueries({ queryKey: ['my-sessions'] });
    },
  });
}

export function useRevokeMySession() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: (sessionId: string) => api.del(`/auth/sessions/${sessionId}`).then(assertSuccess),
    meta: { success: () => t('toast.sessionRevoked') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['my-sessions'] });
    },
  });
}

export function useRevokeAllUserSessions() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: ({ userId, includeSelf = false }: { userId: string; includeSelf?: boolean }) => {
      const path = includeSelf
        ? `/users/${userId}/sessions/revoke-all?includeSelf=true`
        : `/users/${userId}/sessions/revoke-all`;
      return api.post<{ revoked: number }>(path).then(assertSuccess);
    },
    meta: { success: () => t('toast.allSessionsRevoked') },
    onSuccess: (_, vars) => {
      void queryClient.invalidateQueries({ queryKey: ['user-sessions', vars.userId] });
    },
  });
}

export function useRevokeAllMySessions() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('security');
  return useMutation({
    mutationFn: () => api.post<{ revoked: number }>('/auth/sessions/revoke-all').then(assertSuccess),
    meta: { success: () => t('toast.allSessionsRevoked') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['my-sessions'] });
    },
  });
}
