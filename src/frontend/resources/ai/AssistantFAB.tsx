// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSparkles } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useAIContexts } from '@resources/ai';
import { openAssistant } from '@resources/ai/openAssistant';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';

/**
 * Floating "Ask AI" action button that opens the global AI assistant
 * with a starter prompt scoped to the currently-focused entity.
 *
 * Pages that mount `<AssistantFAB />` get a small bottom-right FAB that:
 *   1. Reads the focused `AIContext` (highest priority, registered via
 *      `useAIContext`) so it knows which entity the user is looking at.
 *   2. Builds an "Explain this" prompt for that entity kind (containers,
 *      stacks, images, …).
 *   3. Dispatches the global `mcharbor:open-ai-assistant` event with the
 *      prompt payload. The dialog is owned by `AppLayout`; this component
 *      is just a trigger.
 *
 * Hiding behaviour: when the focused context has no resourceId (the user
 * is on a list / dashboard page), the FAB falls back to a generic prompt
 * but still renders. Pass `enabled={false}` to hide it explicitly (e.g.
 * while data is loading).
 *
 * Prompt templates live in `core/i18n/locales/<lang>/ai.json` under
 * `fab.prompts.<kind>` so they re-translate when the admin switches
 * language. Keys resolve through `useTranslation('ai')` at render time.
 */

const PROMPT_KEY_BY_KIND: Record<string, string> = {
  container: 'fab.prompts.container',
  stack: 'fab.prompts.stack',
  image: 'fab.prompts.image',
  volume: 'fab.prompts.volume',
  network: 'fab.prompts.network',
  pod: 'fab.prompts.pod',
  deployment: 'fab.prompts.deployment',
  workflow: 'fab.prompts.workflow',
  environment: 'fab.prompts.environment',
};

export interface AssistantFABProps {
  /** Hide the FAB (e.g. while data is loading or feature is disabled). */
  enabled?: boolean;
  /** Override the generated prompt for the focused entity. */
  prompt?: string;
  /** Optional className override for layout adjustments. */
  className?: string;
}

export function AssistantFAB({ enabled = true, prompt: promptText, className }: AssistantFABProps) {
  const { t } = useTranslation('ai');
  const { focused } = useAIContexts();

  const handleClick = useCallback(() => {
    if (promptText) {
      openAssistant(promptText);
      return;
    }
    const key =
      (focused.resourceId && focused.type ? PROMPT_KEY_BY_KIND[focused.type] : null) ?? 'fab.prompts.fallback';
    openAssistant(t(key));
  }, [focused.resourceId, focused.type, promptText, t]);

  if (!enabled) return null;

  const tooltip = focused.resourceName
    ? t('aiAssistant.fab.tooltipScoped', { name: focused.resourceName })
    : t('aiAssistant.fab.tooltip');

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="default"
          size="icon"
          onClick={handleClick}
          aria-label={tooltip}
          className={cn(
            'fixed right-6 bottom-6 z-40 inline-flex size-12 items-center justify-center border border-border bg-primary text-primary-foreground shadow-lg transition-all hover:scale-105 hover:shadow-xl focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none',
            className,
          )}
          data-testid="assistant-fab"
        >
          <IconSparkles className="size-5" aria-hidden />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">{tooltip}</TooltipContent>
    </Tooltip>
  );
}
