// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBolt, IconRobot, IconUser } from '@tabler/icons-react';
import { Button } from './ui/Button';
import { Badge } from './ui/Badge';
import { Card } from './ui/Card';
import { Spinner } from './ui/Spinner';
import { Flex } from './ui/Layout';
import { Text as UiText } from './ui/Typography';
import { Markdown } from './Markdown';
import { cn } from '@resources/utils/cn';

export type AIAssistanceMessage = {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  pending?: boolean;
  createdAt: number;
};

type AIActionsBannerProps = {
  count: number;
  busy: boolean;
  onReview: () => void;
};

export function AIActionsBanner({ count, busy, onReview }: AIActionsBannerProps) {
  const { t } = useTranslation(['ai', 'common']);
  return (
    <Card className="border-primary/30 bg-primary/5 p-3 shadow-none">
      <Flex align="center" justify="between" gap={3} wrap>
        <Flex align="center" gap={1.5} className="min-w-0">
          <IconBolt className="size-3.5 shrink-0 text-primary" />
          <UiText size="xs" weight="semibold" className="tracking-wider uppercase">
            {t('aiAssistant.actions.awaitingApproval')}
          </UiText>
          <Badge variant="warning" className="px-2 py-1 text-[10px]">
            {count}
          </Badge>
        </Flex>
        <Button type="button" size="sm" onClick={onReview} disabled={busy}>
          {t('aiAssistant.actions.review')}
        </Button>
      </Flex>
    </Card>
  );
}

export function AIMessageList({ messages }: { messages: AIAssistanceMessage[] }) {
  const { t } = useTranslation(['ai', 'common']);
  return (
    <>
      {messages.map((m) =>
        m.role === 'system' ? (
          <Flex key={m.id} align="center" justify="center">
            <UiText variant="muted" size="xs" className="border border-border bg-muted/40 px-3 py-1">
              {m.content}
            </UiText>
          </Flex>
        ) : (
          <div key={m.id} className={cn('flex gap-2', m.role === 'user' ? 'justify-end' : 'justify-start')}>
            {m.role === 'assistant' && (
              <span className="flex size-7 shrink-0 items-center justify-center bg-primary/10 text-primary">
                <IconRobot className="size-4" />
              </span>
            )}
            <div
              className={cn(
                'max-w-[80%] rounded-lg px-3 py-2 text-sm',
                m.role === 'user'
                  ? 'bg-primary text-primary-foreground'
                  : 'border border-border bg-card text-foreground',
              )}
            >
              <span className="sr-only">
                {m.role === 'user' ? t('aiAssistant.userMessageLabel') : t('aiAssistant.assistantMessageLabel')}
              </span>
              {m.pending ? (
                <Flex align="center" gap={2} className="text-muted-foreground">
                  <Spinner size="sm" />
                  <UiText variant="muted" className="text-xs">
                    {t('aiAssistant.thinking')}
                  </UiText>
                </Flex>
              ) : m.role === 'user' ? (
                <div className="whitespace-pre-wrap">{m.content}</div>
              ) : (
                <Markdown source={m.content} className="mcp-markdown-body" />
              )}
            </div>
            {m.role === 'user' && (
              <span className="flex size-7 shrink-0 items-center justify-center bg-muted text-muted-foreground">
                <IconUser className="size-4" />
              </span>
            )}
          </div>
        ),
      )}
    </>
  );
}
