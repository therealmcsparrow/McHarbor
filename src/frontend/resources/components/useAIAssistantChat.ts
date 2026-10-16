// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAIChat, AIChatError, type AIChatResponse } from '@resources/hooks/useAIChat';
import type { AIContext } from '@resources/ai';
import { useEnvironmentStore } from '@resources/stores/environment';
import { usePromptHistory } from '@resources/hooks/usePromptHistory';
import type { AIAssistanceMessage } from './AIAssistantMessages';

async function askAssistant(
  context: AIContext | undefined,
  question: string,
  conversationId: string | undefined,
  environmentId: string | undefined,
  chat: ReturnType<typeof useAIChat>,
  connectionId: string,
): Promise<{ response: AIChatResponse | null; errorCode?: string }> {
  try {
    const response = await chat.mutateAsync({
      conversationId,
      message: question,
      // Empty is meaningful: the backend reads it as "estate default".
      connectionId: connectionId || undefined,
      environmentId: context?.type === 'environment' ? context.resourceId : environmentId,
      containerId: context?.type === 'container' ? context.resourceId : undefined,
    });
    return { response: response ?? null };
  } catch (err) {
    return {
      response: null,
      errorCode: err instanceof AIChatError ? err.code : undefined,
    };
  }
}

type ChatSessionArgs = {
  isOpen: boolean;
  focused: AIContext;
  connectionId: string;
  afterSend: () => Promise<unknown>;
  seedPrompt?: string | null;
  onSeedConsumed?: () => void;
};

export function useAIAssistantChat({
  isOpen,
  focused,
  connectionId,
  afterSend,
  seedPrompt,
  onSeedConsumed,
}: ChatSessionArgs) {
  const { t } = useTranslation(['ai', 'common']);
  const chat = useAIChat();
  const [messages, setMessages] = useState<AIAssistanceMessage[]>([]);
  const [conversationId, setConversationId] = useState<string>();
  const [input, setInput] = useState('');
  const [pending, setPending] = useState(false);
  const environmentId = useEnvironmentStore((state) => state.currentId);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const promptHistory = usePromptHistory({ value: input, setValue: setInput, inputRef });
  // Monotonic suffix: two consent messages can land inside the same millisecond.
  const systemMessageSeq = useRef(0);

  const pushSystemMessage = (content: string) => {
    systemMessageSeq.current += 1;
    setMessages((prev) => [
      ...prev,
      {
        id: `m-${Date.now()}-s-${systemMessageSeq.current}`,
        role: 'system',
        content,
        createdAt: Date.now(),
      },
    ]);
  };

  // Apply the seed prompt once on open. Subsequent opens start blank
  // because the parent clears `seedPrompt` after the first consume.
  useEffect(() => {
    if (!isOpen) return;
    if (seedPrompt && seedPrompt.trim().length > 0) {
      setInput(seedPrompt);
      onSeedConsumed?.();
    }
    // Only react to (open, seedPrompt) — input changes don't matter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, seedPrompt]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
  }, [messages]);

  const handleSend = async (text?: string) => {
    const question = (text ?? input).trim();
    if (!question || pending) return;

    promptHistory.remember(question);
    setInput('');
    setPending(true);
    setMessages((prev) => [
      ...prev,
      { id: `m-${Date.now()}-u`, role: 'user', content: question, createdAt: Date.now() },
      {
        id: `m-${Date.now()}-a-pending`,
        role: 'assistant',
        content: '',
        pending: true,
        createdAt: Date.now(),
      },
    ]);

    const { response, errorCode } = await askAssistant(
      focused,
      question,
      conversationId,
      environmentId,
      chat,
      connectionId,
    );
    if (response) setConversationId(response.conversationId);
    await afterSend();
    setMessages((prev) => {
      const next = [...prev];
      const lastPending = next.findIndex((m) => m.pending);
      if (lastPending >= 0 && response) {
        next[lastPending] = {
          id: response.assistant.id,
          role: 'assistant',
          content: response.assistant.content,
          createdAt: Date.now(),
        };
      } else if (lastPending >= 0) {
        next[lastPending] = {
          id: `m-${Date.now()}-a-err`,
          role: 'assistant',
          content:
            errorCode === 'err.ai.rate_limited' ? t('aiAssistant.rateLimited') : t('aiAssistant.backendUnavailable'),
          createdAt: Date.now(),
        };
      }
      return next;
    });
    setPending(false);
  };

  const reset = () => {
    setMessages([]);
    setInput('');
    setConversationId(undefined);
    promptHistory.clear();
  };

  return {
    messages,
    setMessages,
    input,
    setInput,
    pending,
    scrollRef,
    inputRef,
    promptHistory,
    pushSystemMessage,
    handleSend,
    reset,
  };
}
