// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { AIProviderOAuth, AIProviderOAuthDefaults } from './useAIProviderConnections';

// "Sign in to connect" — the OAuth 2.0 device flow (RFC 8628) that is
// the alternative to a pasted API key on an AI provider connection.
//
// Three calls make the whole flow: begin asks the provider for a short
// user code, poll waits for the operator to approve it on the
// provider's own site, disconnect drops the stored token. The device
// code itself never reaches the browser — only the user code and the
// verification URI do.

const CONNECTIONS_KEY = ['aiProviderConnections'] as const;
const DEFAULT_KEY = ['aiProviderConnections', 'default'] as const;

export const aiProviderOAuthKey = (id: string) => ['aiProviderConnections', id, 'oauth'] as const;

export const aiProviderOAuthDefaultsKey = ['aiProviderConnections', 'oauth-defaults'] as const;

/** Device-flow prefills keyed by provider type: the host's published
 *  endpoints, overlaid with any client ID + endpoints a plugin declares.
 *  The backend owns the merge so the two cannot drift; the create dialog
 *  prefills the endpoint fields for known providers and, when a plugin
 *  ships a complete config (`pluginProvided`), offers one-click sign-in. */
export function useAIProviderOAuthDefaults() {
  return useQuery({
    queryKey: aiProviderOAuthDefaultsKey,
    queryFn: () =>
      api.get<Record<string, AIProviderOAuthDefaults>>('/ai-providers/oauth-defaults').then((r) => r.data ?? {}),
    // Compiled into the binary — refetching it per dialog open is waste.
    staleTime: Infinity,
  });
}

/** Live sign-in state for one connection. `enabled` lets the caller
 *  skip the request for API-key connections, and `refetchInterval`
 *  drives the pending-code countdown without a second hook. */
export function useAIProviderOAuthStatus(id: string | null, enabled = true) {
  return useQuery({
    queryKey: aiProviderOAuthKey(id ?? ''),
    queryFn: () => api.get<AIProviderOAuth>(`/ai-providers/${id}/oauth/status`).then((r) => r.data ?? null),
    enabled: !!id && enabled,
  });
}

export function useBeginAIProviderOAuth() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<AIProviderOAuth>(`/ai-providers/${id}/oauth/begin`, {}).then(assertSuccess),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: aiProviderOAuthKey(id) });
    },
  });
}

/** One poll tick. The panel calls this on the interval the provider
 *  asked for; a still-pending flow comes back as a success with
 *  `pending: true`, not an error, so no toast fires while waiting. */
export function usePollAIProviderOAuth() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<AIProviderOAuth>(`/ai-providers/${id}/oauth/poll`, {}).then(assertSuccess),
    onSuccess: (data, id) => {
      queryClient.setQueryData(aiProviderOAuthKey(id), data);
      if (data?.connected) {
        void queryClient.invalidateQueries({ queryKey: CONNECTIONS_KEY });
        void queryClient.invalidateQueries({ queryKey: DEFAULT_KEY });
      }
    },
  });
}

export function useDisconnectAIProviderOAuth() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (id: string) => api.post(`/ai-providers/${id}/oauth/disconnect`, {}).then(assertSuccess),
    meta: { success: () => t('toast.aiProviderSignedOut') },
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: aiProviderOAuthKey(id) });
      void queryClient.invalidateQueries({ queryKey: CONNECTIONS_KEY });
      void queryClient.invalidateQueries({ queryKey: DEFAULT_KEY });
    },
  });
}

/**
 * The whole sign-in flow for one connection, as a single hook: current
 * state, a begin action, the poll loop while a code is outstanding, and
 * disconnect. The panel that renders it stays presentational.
 *
 * The loop runs on the interval the provider asked for (RFC 8628 tells
 * clients to honour it — polling faster earns a `slow_down`), and it
 * only runs while a code is actually pending, so a signed-in or
 * unconfigured connection costs nothing.
 */
export function useAIProviderOAuthFlow(id: string | null, enabled = true) {
  const statusQuery = useAIProviderOAuthStatus(id, enabled);
  const begin = useBeginAIProviderOAuth();
  const poll = usePollAIProviderOAuth();
  const disconnect = useDisconnectAIProviderOAuth();

  const currentStatus = statusQuery.data ?? null;
  const pending = !!currentStatus?.pending;
  const pollMs = Math.max(currentStatus?.intervalSeconds ?? 5, 1) * 1000;
  const pollMutate = poll.mutate;

  useEffect(() => {
    if (!id || !pending) return;
    const timer = window.setInterval(() => pollMutate(id), pollMs);
    return () => window.clearInterval(timer);
  }, [id, pending, pollMs, pollMutate]);

  return {
    status: currentStatus,
    pending,
    connected: !!currentStatus?.connected,
    isLoading: statusQuery.isLoading,
    isStarting: begin.isPending,
    isPolling: poll.isPending,
    isDisconnecting: disconnect.isPending,
    start: () => {
      if (id) begin.mutate(id);
    },
    disconnect: () => {
      if (id) disconnect.mutate(id);
    },
  };
}
