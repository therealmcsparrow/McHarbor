// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconCopy, IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { ComputeRequestError } from '../hooks/useCompute';
import type { CapabilityItem } from '../types';
import { CapabilityInstall } from './CapabilityInstall';

/**
 * Why the last check did not finish.
 *
 * The message is the backend's own i18n code — `err.compute.agent_failed` and
 * friends — resolved against this plugin's namespace, so an operator reads the
 * reason rather than inferring it from a button that stopped spinning. The
 * server log still holds the host-specific detail; the code names the class of
 * failure, which is what decides whether the next move is "retry" or "go fix
 * the host".
 */
export function ProbeFailure({ error, className }: { error: Error | null; className?: string }) {
  const { t } = useTranslation('compute');

  const code = error instanceof ComputeRequestError ? error.code : undefined;
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: the backend err.compute.* code set is open-ended
  const detail = code ? t(code, { defaultValue: t('capability.probeFailedRetry') }) : t('capability.probeFailedRetry');

  return <Alert variant="destructive" className={className} title={t('capability.probeFailed')} description={detail} />;
}

/** One checklist line. */
export function CapabilityRow({ item, environmentId }: { item: CapabilityItem; environmentId: string }) {
  const { t } = useTranslation('compute');
  const [copied, setCopied] = useState(false);

  // The key maps to a translated label; the detail and remediation are
  // host-specific text produced by the probe and travel as-is.
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: probe item keys are emitted by the agent and can outgrow the bundle
  const label = t(`capability.items.${item.key}`, { defaultValue: item.key });

  const copy = async () => {
    if (!item.remediation) return;
    try {
      await navigator.clipboard.writeText(item.remediation);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard access can be denied; the command is visible either way, so
      // there is nothing worth interrupting the operator about.
    }
  };

  return (
    <li className="flex items-start gap-3 p-4">
      <span className="mt-0.5 shrink-0" aria-hidden="true">
        {item.present ? (
          <IconCheck className="h-5 w-5 text-success" />
        ) : item.critical ? (
          <IconX className="h-5 w-5 text-destructive" />
        ) : (
          <IconAlertTriangle className="h-5 w-5 text-warning" />
        )}
      </span>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-foreground">{label}</span>
          <span className="sr-only">{item.present ? t('capability.present') : t('capability.missing')}</span>
          {!item.present && item.critical && <Badge variant="destructive">{t('capability.critical')}</Badge>}
          {!item.present && item.blocks && <Badge variant="outline">{t(`capability.blocks.${item.blocks}`)}</Badge>}
          {!item.present && item.installable && (
            <CapabilityInstall environmentId={environmentId} item={item} label={label} />
          )}
        </div>

        {item.detail && <p className="mt-1 text-sm text-muted-foreground">{item.detail}</p>}

        {!item.present && item.remediation && (
          <div className="mt-2 flex items-start gap-2">
            <code className="min-w-0 flex-1 overflow-x-auto rounded bg-muted px-2 py-1.5 font-mono text-xs text-foreground">
              {item.remediation}
            </code>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon" aria-label={t('actions.copyCommand')} onClick={copy}>
                  {copied ? (
                    <IconCheck className="h-4 w-4 text-success" aria-hidden="true" />
                  ) : (
                    <IconCopy className="h-4 w-4" aria-hidden="true" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t('actions.copyCommand')}</TooltipContent>
            </Tooltip>
          </div>
        )}
      </div>
    </li>
  );
}
