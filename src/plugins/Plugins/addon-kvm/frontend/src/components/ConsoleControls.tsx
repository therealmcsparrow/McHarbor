// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconPlugConnected, IconPlugConnectedX, IconRefresh, IconUsers } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { VM_PERM } from '../permissions';
import type { ConsoleStatus } from '../console-sessions';

export function ConsoleStatusBadge({ state }: { state: ConsoleStatus }) {
  const { t } = useTranslation('vm');
  switch (state) {
    case 'connected':
      return <Badge variant="success">{t('console.connected')}</Badge>;
    case 'connecting':
      return <Badge variant="secondary">{t('console.connecting')}</Badge>;
    case 'error':
    case 'disconnected':
      return <Badge variant="outline">{t('console.disconnected')}</Badge>;
    default:
      return null;
  }
}

/**
 * The console has no multi-seat awareness: a second cursor moving on its own
 * with nothing to explain it is confusing, so the count is shown.
 */
export function ConsoleViewersBadge({ viewers }: { viewers: number }) {
  const { t } = useTranslation('vm');
  if (viewers <= 1) return null;
  return (
    <Badge variant="warning">
      <IconUsers className="h-3 w-3" aria-hidden="true" />
      {t('console.viewers', { count: viewers })}
    </Badge>
  );
}

export function ConsoleConnectButtons({
  state,
  live,
  onConnect,
  onDisconnect,
}: {
  state: ConsoleStatus;
  live: boolean;
  onConnect: () => void;
  onDisconnect: () => void;
}) {
  const { t } = useTranslation('vm');
  return (
    <>
      {live && (
        <Button size="sm" variant="outline" permission={VM_PERM.CONSOLE} onClick={onDisconnect}>
          <IconPlugConnectedX className="mr-2 h-4 w-4" aria-hidden="true" />
          {t('console.disconnect')}
        </Button>
      )}
      <Button size="sm" permission={VM_PERM.CONSOLE} onClick={onConnect} disabled={state === 'connecting'}>
        {state === 'connecting' ? (
          <Spinner className="mr-2 h-4 w-4" />
        ) : state === 'connected' ? (
          <IconRefresh className="mr-2 h-4 w-4" aria-hidden="true" />
        ) : (
          <IconPlugConnected className="mr-2 h-4 w-4" aria-hidden="true" />
        )}
        {state === 'connected' ? t('console.reconnect') : t('actions.console')}
      </Button>
    </>
  );
}

export function ConsoleErrorLine({ state, detail }: { state: ConsoleStatus; detail: string }) {
  const { t } = useTranslation('vm');
  if (state !== 'error') return null;
  return (
    <div className="mb-3 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2">
      <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
      <div>
        <p className="text-sm text-destructive">{t('console.disconnected')}</p>
        {detail && <p className="mt-0.5 text-xs text-muted-foreground">{detail}</p>}
      </div>
    </div>
  );
}
