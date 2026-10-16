// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Global event used to open the AI assistant dialog.
 *
 * The dialog lives at the root of `AppLayout`. Any code path in the
 * app can open it by dispatching this event on `window`. The dialog
 * is responsible for translating the optional `prompt` into a
 * pre-filled input box.
 *
 * Consumers:
 *   - `Header.tsx` (the "AI assistant" header button)
 *   - `AssistantFAB.tsx` (the per-page FAB)
 *   - any future quick-action that wants to summon the assistant
 */
export const OPEN_AI_ASSISTANT_EVENT = 'mcharbor:open-ai-assistant';

export interface OpenAIAssistantDetail {
  /**
   * Optional starter prompt. When provided, the dialog pre-fills
   * its input box so the user can either edit it or hit Send
   * immediately.
   */
  prompt?: string;
}

/**
 * Imperative helper to open the AI assistant. Safe to call from
 * anywhere; no-op in test environments where `window` is undefined.
 */
export function openAssistant(promptText?: string): void {
  if (typeof window === 'undefined') return;
  const detail: OpenAIAssistantDetail = {};
  if (promptText) detail.prompt = promptText;
  window.dispatchEvent(new CustomEvent<OpenAIAssistantDetail>(OPEN_AI_ASSISTANT_EVENT, { detail }));
}
