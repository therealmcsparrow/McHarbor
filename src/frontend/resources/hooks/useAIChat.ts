// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

// === AI assistant hooks ===
//
// These hooks live in the new `ai_assistant` frontend module (mirrors
// the backend `modules/ai_assistant` package). They were previously
// grouped with container/image hooks in
// `containers/hooks/useContainerAdvanced2.ts`, forcing settings and
// dialogs to import AI APIs from the containers module.

export type AIChatRequest = {
  conversationId?: string;
  environmentId?: string;
  containerId?: string;
  message: string;
  /**
   * Pins the request to one `ai_provider_connections` row. Omitted means
   * the estate default, which is what every caller did before the chat
   * gained a provider picker.
   *
   * A *connection* id, not a provider type: one plugin can back several
   * connections (a prod and a dev OpenAI key are two rows of
   * provider_type "openai"), and a type could not tell them apart.
   */
  connectionId?: string;
};

export type AIChatMessage = {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  createdAt: string;
};

export type AIChatResponse = {
  conversationId: string;
  userMessage: AIChatMessage;
  assistant: AIChatMessage;
  suggestions?: string[];
};

export type AIActionStatus = 'pending' | 'approved' | 'executed' | 'failed' | 'denied' | 'expired';

export type AIAction = {
  id: string;
  conversationId: string;
  userId: string;
  tool: string;
  argsJson?: string;
  planJson?: string;
  resultJson?: string;
  consentAt?: string;
  executedAt: string;
  idempotencyKey?: string;
  status: AIActionStatus;
  error?: string;
};

export class AIChatError extends Error {
  readonly code?: string;

  constructor(message: string, code?: string) {
    super(message);
    this.name = 'AIChatError';
    this.code = code;
  }
}

export function useAIChat() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: (payload: AIChatRequest) =>
      api.post<AIChatResponse>('/ai-assistant/chat', payload).then((r) => {
        if (!r.success) throw new AIChatError(r.error ?? 'Request failed', r.code);
        return r.data;
      }),
    meta: { error: t('toast.error') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['ai-conversations'] });
      void queryClient.invalidateQueries({ queryKey: ['ai-actions'] });
    },
  });
}

export function useAIActions(enabled = true) {
  return useQuery({
    queryKey: ['ai-actions'],
    enabled,
    queryFn: () => api.get<AIAction[]>('/ai-assistant/actions', { limit: '20' }).then((r) => r.data ?? []),
    refetchInterval: (q) => (q.state.data?.some((a) => a.status === 'pending') ? 5000 : false) as number,
  });
}

export function useConsentAIAction() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: ({ id, approve }: { id: string; approve: boolean }) =>
      api.post<AIAction>(`/ai-assistant/actions/${id}/consent`, { approve }).then(assertSuccess),
    meta: { error: t('toast.error') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['ai-actions'] });
    },
  });
}

export function useExecuteAIAction() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: (id: string) => api.post<AIAction>(`/ai-assistant/actions/${id}/execute`, {}).then(assertSuccess),
    meta: { error: t('toast.error') },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['ai-actions'] });
    },
  });
}
