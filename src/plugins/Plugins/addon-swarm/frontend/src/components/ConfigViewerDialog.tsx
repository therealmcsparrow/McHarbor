// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Spinner } from '@resources/components/ui/Spinner';
import { useSwarmConfig } from '../hooks/useSwarmSecrets';
import { formatBytes } from '../format';
import type { SwarmConfig } from '../types';

type ConfigViewerDialogProps = {
  config: SwarmConfig | null;
  onOpenChange: (open: boolean) => void;
};

/**
 * Show a config's contents.
 *
 * The payload is fetched only when this dialog opens, from the single-config
 * endpoint — the list response deliberately carries no `data`, so there is
 * nothing here to render until the read happens.
 *
 * There is no equivalent for a secret, and there cannot be: Docker's secret
 * API never returns the value.
 */
export function ConfigViewerDialog({ config, onOpenChange }: ConfigViewerDialogProps) {
  const { t } = useTranslation('swarm');
  const query = useSwarmConfig(config?.id, config !== null);

  const decoded = query.data?.data ? fromBase64(query.data.data) : '';

  return (
    <Dialog open={config !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{config?.name ?? ''}</DialogTitle>
          <DialogDescription>
            {t('dialogs.configViewer.description', {
              size: formatBytes(query.data?.size ?? config?.size ?? 0),
            })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {query.isLoading && <Spinner size="sm" />}
          {query.error && <ErrorState title={t('errors.configReadFailed')} error={query.error} />}
          {query.data && (
            <pre className="max-h-[55vh] overflow-auto rounded-lg bg-muted p-3 font-mono text-xs text-foreground">
              {decoded}
            </pre>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => void navigator.clipboard?.writeText(decoded)} disabled={!decoded}>
            <IconCopy className="size-4" />
            {t('actions.copy')}
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * UTF-8 safe base64 decode.
 *
 * A binary config decoded as text is unreadable either way, but this at least
 * renders a UTF-8 one correctly instead of mangling every non-ASCII byte.
 */
function fromBase64(input: string): string {
  try {
    const binary = atob(input);
    const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
    return new TextDecoder().decode(bytes);
  } catch {
    return input;
  }
}
