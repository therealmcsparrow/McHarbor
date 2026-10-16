// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBolt, IconInfoCircle, IconRefresh, IconSparkles } from '@tabler/icons-react';
import { DialogDescription, DialogHeader, DialogTitle } from './ui/Dialog';
import { Button } from './ui/Button';
import { Box, Flex } from './ui/Layout';
import { Text as UiText } from './ui/Typography';
import { Select } from '@resources/components/ui/Select';
import type { AIContext } from '@resources/ai';
import type { AIProviderPicker } from './useAIProviderPicker';

type AIAssistantHeaderProps = {
  picker: AIProviderPicker;
  focused: AIContext;
  providerCount: number;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onReset: () => void;
};

export function AIAssistantHeader({
  picker,
  focused,
  providerCount,
  pending,
  onOpenChange,
  onReset,
}: AIAssistantHeaderProps) {
  const { t } = useTranslation(['ai', 'common']);
  const { connectionId, setConnectionId, enabledConnections, connectionOptions, defaultConnection } = picker;
  return (
    <DialogHeader>
      <Flex align="center" gap={2}>
        <IconSparkles className="size-5 text-primary" />
        <DialogTitle>{t('aiAssistant.title')}</DialogTitle>
        <DialogDescription className="sr-only">{t('aiAssistant.description')}</DialogDescription>
        <Box className="ml-auto flex items-center gap-1">
          {/*
            Provider picker. Only rendered with more than one enabled
            connection: with a single provider the control would be a
            dropdown of one, which reads as a choice while offering
            none. Disabled connections are filtered out because the
            backend refuses them, so listing one would only produce a
            failed send.
          */}
          {enabledConnections.length > 1 && (
            <Select
              value={connectionId}
              onChange={setConnectionId}
              options={connectionOptions}
              ariaLabel={t('aiAssistant.providerLabel')}
              variant="outline"
              className="w-44"
            />
          )}
          {/*
            The complement to the rule above: with exactly one provider the
            picker is correctly hidden, which left nothing on screen saying
            *which* model answers. This names the default instead - a
            label, not a control, so it offers no false choice.
          */}
          {enabledConnections.length === 1 && defaultConnection && (
            <UiText variant="muted" size="sm" className="max-w-44 truncate" title={defaultConnection.name}>
              {defaultConnection.name}
            </UiText>
          )}
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => {
              onOpenChange(false);
              window.location.assign('/settings?tab=ai');
            }}
            aria-label={t('aiAssistant.settings.title')}
          >
            <IconBolt className="size-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onReset}
            disabled={pending}
            aria-label={t('aiAssistant.reset')}
          >
            <IconRefresh className="size-3.5" />
          </Button>
        </Box>
      </Flex>
      <Flex align="center" gap={2} className="mt-2 text-xs text-muted-foreground">
        <IconInfoCircle className="size-3.5" />
        <span>
          {t('aiAssistant.focusedOn', {
            name: focused.resourceName ?? focused.source,
          })}
        </span>
        {providerCount > 0 && (
          <span className="bg-primary/10 px-2 py-0.5 text-[10px] text-primary">
            {t('aiAssistant.providersActive', { count: providerCount })}
          </span>
        )}
      </Flex>
    </DialogHeader>
  );
}
