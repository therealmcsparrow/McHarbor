// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconShieldCheck, IconShieldExclamation } from '@tabler/icons-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { cn } from '@resources/utils/cn';

const MODE_KEYS = {
  pinned: 'tls.mode.pinned',
  verified: 'tls.mode.verified',
  encrypted: 'tls.mode.encrypted',
  insecure: 'tls.mode.insecure',
  plaintext: 'tls.mode.plaintext',
} as const;

const SECURE_MODES = new Set<string>(['pinned', 'verified', 'encrypted']);

type AgentTlsShieldProps = {
  mode?: string;
  className?: string;
};

export function AgentTlsShield({ mode, className }: AgentTlsShieldProps) {
  const { t } = useTranslation('environments');

  const descriptionKey = mode ? MODE_KEYS[mode as keyof typeof MODE_KEYS] : undefined;
  if (!descriptionKey) return null;

  const secure = SECURE_MODES.has(mode as string);
  const Icon = secure ? IconShieldCheck : IconShieldExclamation;
  const description = t(descriptionKey);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn('inline-flex shrink-0 items-center', secure ? 'text-success' : 'text-warning', className)}>
          <Icon className="size-4" aria-hidden="true" />
          <span className="sr-only">{`${t('tls.title')}: ${description}`}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="font-medium">{t('tls.title')}</p>
        <p className="text-muted-foreground">{description}</p>
      </TooltipContent>
    </Tooltip>
  );
}
