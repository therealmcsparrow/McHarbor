// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBolt, IconCheck, IconPlayerPlay, IconX } from '@tabler/icons-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from './ui/Dialog';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from './ui/Button';
import { Badge } from './ui/Badge';
import { Box, Flex } from './ui/Layout';
import { Text as UiText } from './ui/Typography';
import type { AIAction } from '@resources/hooks/useAIChat';

type AIActionApprovalDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Actions still awaiting consent or execution, newest first. */
  actions: AIAction[];
  /** True while a consent / execute mutation is in flight. */
  busy?: boolean;
  onApprove: (action: AIAction) => void;
  onDeny: (action: AIAction) => void;
};

export function formatActionPayload(value?: string): string {
  if (!value) return '';
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

export function actionStatusVariant(
  currentStatus: AIAction['status'],
): 'default' | 'success' | 'warning' | 'destructive' | 'secondary' {
  if (currentStatus === 'executed') return 'success';
  if (currentStatus === 'failed' || currentStatus === 'denied' || currentStatus === 'expired') return 'destructive';
  if (currentStatus === 'approved') return 'warning';
  return 'default';
}

/**
 * Consent gate for AI-proposed actions. It is deliberately a modal rather than
 * a card inside the chat log: the assistant is asking for permission to change
 * something, so the question has to reach the user instead of scrolling past.
 */
export function AIActionApprovalDialog({
  open: isOpen,
  onOpenChange,
  actions,
  busy = false,
  onApprove,
  onDeny,
}: AIActionApprovalDialogProps) {
  const { t } = useTranslation(['ai', 'common']);

  // One decision at a time: the buttons live in the footer, so the modal can
  // only ever be answering for a single action. The queue drains head-first.
  const action = actions[0];
  if (!action) return null;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <Flex align="center" gap={2}>
            <IconBolt className="size-5 text-primary" />
            <DialogTitle>{t('aiAssistant.actions.title')}</DialogTitle>
            {actions.length > 1 && (
              <Badge variant="warning" className="px-2 py-1 text-[10px]">
                {t('aiAssistant.actions.queuePosition', { index: 1, total: actions.length })}
              </Badge>
            )}
          </Flex>
          <DialogDescription className="mt-1">{t('aiAssistant.actions.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          <Box className="rounded-lg border border-border bg-card p-4">
            <Flex align="center" gap={2} wrap>
              <UiText size="sm" weight="medium" truncate>
                {action.tool}
              </UiText>
              <Badge variant={actionStatusVariant(action.status)} className="px-2 py-1 text-[10px]">
                {t(`aiAssistant.actions.status.${action.status}`)}
              </Badge>
            </Flex>
            {action.error && (
              <UiText size="xs" className="mt-1 text-destructive">
                {action.error}
              </UiText>
            )}
            {(action.planJson || action.argsJson) && (
              <Box className="mt-2">
                <UiText variant="muted" size="xs" className="mb-1 tracking-wider uppercase">
                  {t('aiAssistant.actions.plan')}
                </UiText>
                <Box className="max-h-48 overflow-auto rounded-md border border-border bg-background p-2">
                  <UiText as="pre" size="xs" monospace className="break-words whitespace-pre-wrap">
                    {formatActionPayload(action.planJson || action.argsJson)}
                  </UiText>
                </Box>
              </Box>
            )}
          </Box>
        </DialogBody>

        <DialogFooter>
          {action.status === 'pending' && (
            <Button type="button" variant="outline" onClick={() => onDeny(action)} disabled={busy}>
              <IconX className="size-4" />
              {t('aiAssistant.actions.deny')}
            </Button>
          )}
          <Button type="button" permission={PERM.AI_ASSISTANT_ACT} onClick={() => onApprove(action)} disabled={busy}>
            {action.status === 'approved' ? <IconPlayerPlay className="size-4" /> : <IconCheck className="size-4" />}
            {action.status === 'approved' ? t('aiAssistant.actions.run') : t('aiAssistant.actions.approveAndRun')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
