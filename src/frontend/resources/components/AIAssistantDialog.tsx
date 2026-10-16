// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBolt } from '@tabler/icons-react';
import { Dialog, DialogBody, DialogContent, DialogFooter } from './ui/Dialog';
import { Box } from './ui/Layout';
import { Text as UiText } from './ui/Typography';
import { type AICapability, useAIContexts } from '@resources/ai';
import { CapabilityChips } from '@resources/ai/CapabilityChips';
import { AIActionApprovalDialog } from './AIActionApprovalDialog';
import { AIActionsBanner, AIMessageList } from './AIAssistantMessages';
import { useAIAssistantChat } from './useAIAssistantChat';
import { AIAssistantHeader } from './AIAssistantHeader';
import { AIAssistantForm } from './AIAssistantForm';
import { useAIProviderPicker } from './useAIProviderPicker';
import { useAIActionQueue } from './useAIActionQueue';

type AIAssistantDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /**
   * Optional starter prompt. When the dialog opens with a non-empty
   * `seedPrompt`, it pre-fills the input box. The dialog calls
   * `onSeedConsumed()` once so the parent can clear the seed and the
   * next open starts blank.
   */
  seedPrompt?: string | null;
  onSeedConsumed?: () => void;
};

/**
 * The dialog itself no longer takes a context prop. Every page / addon
 * publishes its context into the central registry via useAIContext(), and
 * the dialog consumes it through useAIContexts().
 */
export function AIAssistantDialog({ open: isOpen, onOpenChange, seedPrompt, onSeedConsumed }: AIAssistantDialogProps) {
  return (
    <AIAssistantDialogInner
      open={isOpen}
      onOpenChange={onOpenChange}
      seedPrompt={seedPrompt}
      onSeedConsumed={onSeedConsumed}
    />
  );
}

function AIAssistantDialogInner({ open: isOpen, onOpenChange, seedPrompt, onSeedConsumed }: AIAssistantDialogProps) {
  const { t } = useTranslation(['ai', 'common']);
  const { contexts, focused, providers } = useAIContexts();
  // NB: `providers` above is *context* providers — the things that
  // contribute page context to a prompt. The model providers are the
  // `ai_provider_connections` rows behind the picker. Two different concepts
  // that unfortunately share a word.
  const picker = useAIProviderPicker();
  const session = useAIAssistantChat({
    isOpen,
    focused,
    connectionId: picker.connectionId,
    afterSend: () => queue.actions.refetch(),
    seedPrompt,
    onSeedConsumed,
  });
  const { messages, setMessages, input, pending, scrollRef } = session;
  const queue = useAIActionQueue(isOpen, session.pushSystemMessage);
  const { activeActions } = queue;

  // All capabilities from all registered pages, deduplicated by id.
  const allCapabilities = useMemo<AICapability[]>(() => {
    const map = new Map<string, AICapability>();
    contexts.forEach((c) => {
      c.capabilities?.forEach((cap) => {
        if (!map.has(cap.id)) map.set(cap.id, cap);
      });
    });
    return Array.from(map.values());
  }, [contexts]);

  const capabilitiesForFocused = focused.capabilities ?? [];

  return (
    <>
      <Dialog open={isOpen} onOpenChange={onOpenChange}>
        <DialogContent className="flex h-[min(600px,75vh)] max-w-2xl flex-col">
          <AIAssistantHeader
            picker={picker}
            focused={focused}
            providerCount={providers.length}
            pending={pending}
            onOpenChange={onOpenChange}
            onReset={session.reset}
          />

          <DialogBody className="p-0">
            <Box ref={scrollRef} className="h-full space-y-3 overflow-y-auto px-4 py-4" role="log" aria-live="polite">
              {activeActions.length > 0 && (
                <AIActionsBanner
                  count={activeActions.length}
                  busy={queue.actionBusy}
                  onReview={() => queue.setApprovalOpen(true)}
                />
              )}
              <AIMessageList messages={messages} />

              {/* Capabilities from registered pages / addons. */}
              {allCapabilities.length > 0 && (
                <div className="rounded-lg border border-border bg-background/30 p-3">
                  <div className="mb-2 flex items-center gap-1.5 text-[10px] font-medium tracking-wider text-muted-foreground uppercase">
                    <IconBolt className="size-3" />
                    {t('aiAssistant.capabilities')}
                  </div>
                  <CapabilityChips
                    capabilities={capabilitiesForFocused.length > 0 ? capabilitiesForFocused : allCapabilities}
                    onInvoke={() => {
                      setMessages((prev) => [
                        ...prev,
                        {
                          id: `m-${Date.now()}-a-action`,
                          role: 'assistant',
                          content: t('aiAssistant.actionDone'),
                          createdAt: Date.now(),
                        },
                      ]);
                    }}
                    disabled={pending}
                  />
                </div>
              )}
            </Box>
          </DialogBody>

          <AIAssistantForm
            input={input}
            setInput={session.setInput}
            inputRef={session.inputRef}
            pending={pending}
            promptHistory={session.promptHistory}
            onSend={() => void session.handleSend()}
          />

          <DialogFooter>
            <UiText variant="muted" className="text-[10px]">
              {t('aiAssistant.footer')}
            </UiText>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      {/* Rendered after the chat dialog on purpose: both overlays sit at z-50,
        so DOM order is what puts the consent modal on top of the chat. */}
      <AIActionApprovalDialog
        open={isOpen && queue.approvalOpen}
        onOpenChange={queue.setApprovalOpen}
        actions={activeActions}
        busy={queue.actionBusy}
        onApprove={(action) => void queue.handleApproveAndRun(action)}
        onDeny={(action) => void queue.handleDeny(action)}
      />
    </>
  );
}
