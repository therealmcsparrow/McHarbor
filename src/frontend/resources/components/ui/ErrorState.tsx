// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useFlash } from '@resources/hooks/useFlash';
import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconCheck, IconCopy } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from './Button';
import { Box, Flex } from './Layout';
import { Caption, Heading, Text as UiText } from './Typography';

type ErrorStateAction = {
  label: string;
  onClick: () => void;
  variant?: 'default' | 'outline' | 'ghost';
};

export type ErrorStateVariant = 'banner' | 'card' | 'inline' | 'page';

export type ErrorStateProps = {
  title?: string;
  description?: string;
  error?: unknown;
  variant?: ErrorStateVariant;
  onRetry?: () => void;
  errorId?: string;
  primaryAction?: ErrorStateAction;
  secondaryAction?: ErrorStateAction;
  className?: string;
};

const variantClasses: Record<ErrorStateVariant, string> = {
  banner: 'flex flex-row items-center gap-3 rounded-lg border border-destructive/40 bg-destructive/5 px-4 py-3',
  card: 'flex flex-col items-center justify-center rounded-lg border border-destructive/40 bg-destructive/5 px-6 py-10 text-center',
  inline: 'flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm',
  page: 'flex flex-col items-center justify-center rounded-xl border border-destructive/40 bg-destructive/5 px-6 py-16 text-center',
};

const iconWrapClasses: Record<ErrorStateVariant, string> = {
  banner: 'flex size-9 shrink-0 items-center justify-center bg-destructive/10 text-destructive',
  card: 'mb-3 flex size-12 items-center justify-center bg-destructive/10 text-destructive',
  inline: 'flex size-5 shrink-0 items-center justify-center text-destructive',
  page: 'mb-4 flex size-14 items-center justify-center bg-destructive/10 text-destructive',
};

const iconClasses: Record<ErrorStateVariant, string> = {
  banner: 'size-4',
  card: 'size-5',
  inline: 'size-4',
  page: 'size-7',
};

function resolveErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function resolveErrorId(error: unknown, errorId?: string): string | undefined {
  if (errorId) return errorId;
  if (typeof error !== 'object' || error === null) return undefined;
  const candidate = Reflect.get(error, 'errorId');
  return typeof candidate === 'string' && candidate ? candidate : undefined;
}

async function copyText(value: string): Promise<void> {
  // No synthetic-textarea fallback: on insecure origins the caller keeps the
  // "copied" state false and the error id stays visible for manual copy.
  if (!navigator.clipboard?.writeText) {
    throw new Error('clipboard unavailable');
  }
  await navigator.clipboard.writeText(value);
}

// i18n: errorState.title, errorState.description, errorState.retry, errorState.copyErrorId, errorState.copied, errorState.errorIdLabel
export function ErrorState({
  title,
  description,
  error,
  variant = 'banner',
  onRetry,
  errorId,
  primaryAction,
  secondaryAction,
  className,
}: ErrorStateProps) {
  const { t } = useTranslation('common');
  const { flashing: copied, flash: flashCopied, clear: clearCopied } = useFlash();
  const resolvedTitle = title ?? t('errorState.title');
  const resolvedDescription = description ?? resolveErrorMessage(error, t('errorState.description'));
  // A caller's own description is context, not a substitute for what the server said.
  const errorMessage = description ? resolveErrorMessage(error, '') : '';
  const cause = errorMessage && errorMessage !== description ? errorMessage : undefined;
  const resolvedErrorId = resolveErrorId(error, errorId);

  const handleCopyErrorId = async () => {
    if (!resolvedErrorId) return;
    try {
      await copyText(resolvedErrorId);
      flashCopied();
    } catch {
      clearCopied();
    }
  };

  const actions = (
    <Flex wrap align="center" justify="center" gap={2} className={variant === 'banner' ? 'ml-auto' : 'mt-4'}>
      {onRetry && (
        <Button type="button" size={variant === 'inline' ? 'sm' : 'default'} onClick={onRetry}>
          {t('errorState.retry')}
        </Button>
      )}
      {resolvedErrorId && variant !== 'inline' && (
        <Button type="button" variant="outline" size="sm" onClick={handleCopyErrorId}>
          {copied ? <IconCheck className="size-3.5" /> : <IconCopy className="size-3.5" />}
          {copied ? t('errorState.copied') : t('errorState.copyErrorId')}
        </Button>
      )}
      {primaryAction && (
        <Button
          type="button"
          variant={primaryAction.variant ?? 'default'}
          size={variant === 'inline' ? 'sm' : 'default'}
          onClick={primaryAction.onClick}
        >
          {primaryAction.label}
        </Button>
      )}
      {secondaryAction && (
        <Button
          type="button"
          variant={secondaryAction.variant ?? 'outline'}
          size={variant === 'inline' ? 'sm' : 'default'}
          onClick={secondaryAction.onClick}
        >
          {secondaryAction.label}
        </Button>
      )}
    </Flex>
  );

  if (variant === 'banner' || variant === 'inline') {
    return (
      <Box className={cn(variantClasses[variant], className)} role="alert" aria-live="polite">
        <Box className={iconWrapClasses[variant]} aria-hidden="true">
          <IconAlertCircle className={iconClasses[variant]} />
        </Box>
        <Box className="min-w-0 flex-1">
          {variant === 'banner' ? (
            <>
              <UiText size="sm" weight="semibold">
                {resolvedTitle}
              </UiText>
              {resolvedDescription && (
                <UiText size="sm" tone="muted" className="break-words">
                  {resolvedDescription}
                </UiText>
              )}
              {cause && (
                <UiText size="sm" className="break-words">
                  {cause}
                </UiText>
              )}
              {resolvedErrorId && (
                <Caption size="xs" tone="muted" className="mt-0.5 block font-mono break-all">
                  {t('errorState.errorIdLabel')}: {resolvedErrorId}
                </Caption>
              )}
            </>
          ) : (
            <UiText size="sm" tone="muted" className="break-words">
              {resolvedTitle}
              {resolvedDescription ? ` — ${resolvedDescription}` : ''}
              {cause ? ` — ${cause}` : ''}
            </UiText>
          )}
        </Box>
        {(onRetry || primaryAction || secondaryAction || (resolvedErrorId && variant === 'banner')) && actions}
      </Box>
    );
  }

  return (
    <Box className={cn(variantClasses[variant], className)} role="alert" aria-live="polite">
      <Box className={iconWrapClasses[variant]} aria-hidden="true">
        <IconAlertCircle className={iconClasses[variant]} />
      </Box>
      <Heading as="h3" size="sm" weight="semibold">
        {resolvedTitle}
      </Heading>
      {resolvedDescription && (
        <UiText size="sm" tone="muted" className="mt-1 max-w-md break-words">
          {resolvedDescription}
        </UiText>
      )}
      {cause && (
        <UiText size="sm" className="mt-1 max-w-xl break-words whitespace-pre-wrap">
          {cause}
        </UiText>
      )}
      {resolvedErrorId && (
        <Caption size="xs" tone="muted" className="mt-2 font-mono break-all">
          {t('errorState.errorIdLabel')}: {resolvedErrorId}
        </Caption>
      )}
      {(onRetry || primaryAction || secondaryAction || resolvedErrorId) && actions}
    </Box>
  );
}
