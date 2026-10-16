// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// AI provider connections: host-owned typed CRUD over named LLM
// provider instances (mirrors communications/storage_locations).
// Each row binds a friendly name + optional base URL + optional
// default model + encrypted API key to a plugin ProviderType so the
// operator can register, e.g., a "prod-openai" and a "dev-openai"
// side-by-side instead of being limited to one openai configuration
// per host instance. The chat composer selects by row id (not by
// ProviderType) so the same plugin can back multiple connections.

/** How a connection authenticates. `api_key` is a pasted key;
 *  `oauth` is an RFC 8628 device-flow sign-in against the provider.
 *  Both are first-class — the form picks one per connection. */
export type AIProviderAuthMethod = 'api_key' | 'oauth';

/** Non-secret half of a connection's device-flow config, plus its
 *  current sign-in state. The client secret and both tokens never
 *  leave the backend; `hasClientSecret` is all the form gets. */
export type AIProviderOAuth = {
  clientId?: string;
  deviceUrl?: string;
  tokenUrl?: string;
  scopes?: string;
  hasClientSecret: boolean;
  connected: boolean;
  accountLabel?: string;
  expiresAt?: string;
  /** A device code is out and waiting for the operator to approve it. */
  pending: boolean;
  userCode?: string;
  verificationUri?: string;
  pendingExpiresAt?: string;
  intervalSeconds?: number;
};

/** Writable half of the device-flow config. An empty `clientSecret`
 *  means "keep the stored one", matching the API key convention. */
export type AIProviderOAuthInput = {
  clientId: string;
  clientSecret: string;
  deviceUrl: string;
  tokenUrl: string;
  scopes: string;
};

/** One entry from GET /ai-providers/oauth-defaults: the non-secret
 *  device-flow prefill for a provider type. `pluginProvided` is true
 *  when the plugin ships a complete config (client ID + both
 *  endpoints), so the operator only clicks Sign in and the form hides
 *  the config fields. Never carries a client secret. */
export type AIProviderOAuthDefaults = {
  clientId?: string;
  deviceUrl?: string;
  tokenUrl?: string;
  scopes?: string;
  pluginProvided?: boolean;
};

export type AIProviderConnection = {
  id: string;
  name: string;
  /** Plugin ProviderType — e.g. "openai", "anthropic", "ollama".
   *  The runtime adapter for this type must be loaded for the
   *  chat dispatch to succeed; the dialog lists installed-and-
   *  enabled plugins as recommended picks. */
  providerType: string;
  isDefault: boolean;
  enabled: boolean;
  baseUrl?: string;
  defaultModel?: string;
  hasApiKey: boolean;
  authMethod: AIProviderAuthMethod;
  /** Present for OAuth connections, and for API-key connections that
   *  still carry a previously configured endpoint set. */
  oauth?: AIProviderOAuth;
  createdAt: string;
  updatedAt: string;
};

export type CreateAIProviderInput = {
  name: string;
  providerType: string;
  isDefault?: boolean;
  enabled?: boolean;
  baseUrl?: string;
  defaultModel?: string;
  apiKey?: string;
  authMethod?: AIProviderAuthMethod;
  oauth?: AIProviderOAuthInput;
};

export type UpdateAIProviderInput = {
  name?: string;
  isDefault?: boolean;
  enabled?: boolean;
  baseUrl?: string;
  defaultModel?: string;
  /** Empty string is treated as "don't touch" so the edit dialog
   *  can leave the field blank when the user has no new key. */
  apiKey?: string;
  authMethod?: AIProviderAuthMethod;
  oauth?: AIProviderOAuthInput;
};

const QUERY_KEY = ['aiProviderConnections'] as const;
const DEFAULT_QUERY_KEY = ['aiProviderConnections', 'default'] as const;

export function useAIProviderConnections() {
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => api.get<AIProviderConnection[]>('/ai-providers').then((r) => r.data ?? []),
  });
}

export function useDefaultAIProviderConnection() {
  return useQuery({
    queryKey: DEFAULT_QUERY_KEY,
    queryFn: () => api.get<AIProviderConnection | null>('/ai-providers/default').then((r) => r.data ?? null),
  });
}

export function useCreateAIProviderConnection() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (body: CreateAIProviderInput) =>
      api.post<AIProviderConnection>('/ai-providers', body).then(assertSuccess),
    meta: { success: () => t('toast.aiProviderCreated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: DEFAULT_QUERY_KEY });
    },
  });
}

export function useUpdateAIProviderConnection() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string } & UpdateAIProviderInput) =>
      api.put<AIProviderConnection>(`/ai-providers/${id}`, body).then(assertSuccess),
    meta: { success: () => t('toast.aiProviderUpdated') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: DEFAULT_QUERY_KEY });
    },
  });
}

export function useDeleteAIProviderConnection() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (id: string) => api.del(`/ai-providers/${id}`).then(assertSuccess),
    meta: { success: () => t('toast.aiProviderDeleted') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: DEFAULT_QUERY_KEY });
    },
  });
}

export function useSetDefaultAIProviderConnection() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('settings');
  return useMutation({
    mutationFn: (id: string) => api.post(`/ai-providers/${id}/default`, {}).then(assertSuccess),
    meta: { success: () => t('toast.aiProviderDefaultSet') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: DEFAULT_QUERY_KEY });
    },
  });
}

export type AIProviderTestStatus = 'pass' | 'warn' | 'fail' | 'skip';

export type AIProviderTestStep = {
  name: string;
  status: AIProviderTestStatus;
  detail?: string;
  latency?: string;
};

export type AIProviderTestResult = {
  connectionId: string;
  connectionName: string;
  providerType: string;
  model?: string;
  overallStatus: AIProviderTestStatus;
  startedAt: string;
  completedAt: string;
  duration: string;
  steps: AIProviderTestStep[];
};

// useTestAIProviderConnection dispatches POST /ai-providers/{id}/test
// and returns the per-step report (load → decrypt → plugin lookup →
// send). Mirrors useTestStorageLocation / useTestChannel: the dialog
// is the source of truth, no toast on success/failure — the operator
// needs the per-step breakdown, not a yes/no.
export function useTestAIProviderConnection() {
  return useMutation({
    mutationFn: (id: string) => api.post<AIProviderTestResult>(`/ai-providers/${id}/test`, {}).then(assertSuccess),
  });
}
