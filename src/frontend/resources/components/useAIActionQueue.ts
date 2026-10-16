// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAIActions, useConsentAIAction, useExecuteAIAction, type AIAction } from '@resources/hooks/useAIChat';

export function useAIActionQueue(isOpen: boolean, pushSystemMessage: (content: string) => void) {
  const { t } = useTranslation(['ai', 'common']);
  const actions = useAIActions(isOpen);
  const consentAction = useConsentAIAction();
  const executeAction = useExecuteAIAction();
  const [approvalOpen, setApprovalOpen] = useState(false);
  // Action ids the approval modal has already been raised for, so dismissing
  // it ("Later") does not immediately re-open on the next actions poll.
  const [announcedActionIds, setAnnouncedActionIds] = useState<string[]>([]);

  const activeActions = useMemo(
    () => (actions.data ?? []).filter((a) => a.status === 'pending' || a.status === 'approved'),
    [actions.data],
  );

  // Raise the modal as soon as an action needs a decision. Only unseen ids
  // re-open it, so a dismissed modal stays dismissed until something new
  // arrives; the queue draining closes it again.
  if (!isOpen) {
    if (announcedActionIds.length > 0) setAnnouncedActionIds([]);
    if (approvalOpen) setApprovalOpen(false);
  } else if (activeActions.length === 0) {
    if (approvalOpen) setApprovalOpen(false);
  } else {
    const unseen = activeActions.filter((a) => !announcedActionIds.includes(a.id));
    if (unseen.length > 0) {
      setAnnouncedActionIds([...announcedActionIds, ...unseen.map((a) => a.id)]);
      setApprovalOpen(true);
    }
  }

  // Both handlers report their outcome back into the chat log, so the
  // conversation carries a record of what the user let the assistant do.
  const handleApproveAndRun = async (action: AIAction) => {
    try {
      const approved =
        action.status === 'approved' ? action : await consentAction.mutateAsync({ id: action.id, approve: true });
      const executed = await executeAction.mutateAsync(approved.id);
      if (executed?.status === 'failed') {
        pushSystemMessage(
          t('aiAssistant.actions.feedback.failed', {
            tool: action.tool,
            error: executed.error || t('aiAssistant.actions.feedback.unknownError'),
          }),
        );
      } else {
        pushSystemMessage(t('aiAssistant.actions.feedback.executed', { tool: action.tool }));
      }
    } catch {
      pushSystemMessage(t('aiAssistant.actions.feedback.error', { tool: action.tool }));
    }
    await actions.refetch();
  };

  const handleDeny = async (action: AIAction) => {
    try {
      await consentAction.mutateAsync({ id: action.id, approve: false });
      pushSystemMessage(t('aiAssistant.actions.feedback.denied', { tool: action.tool }));
    } catch {
      pushSystemMessage(t('aiAssistant.actions.feedback.error', { tool: action.tool }));
    }
    await actions.refetch();
  };

  const actionBusy = consentAction.isPending || executeAction.isPending;

  return {
    actions,
    activeActions,
    approvalOpen,
    setApprovalOpen,
    actionBusy,
    handleApproveAndRun,
    handleDeny,
  };
}
