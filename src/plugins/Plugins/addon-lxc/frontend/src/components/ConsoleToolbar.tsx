// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlugConnected, IconRefresh, IconTerminal2, IconUsers } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { CT_PERM } from '../permissions';
import type { ConsoleStatus } from '../hooks/useConsoleSession';
import type { ConsoleKind } from '../types';

interface ConsoleToolbarProps {
  status: ConsoleStatus;
  kind: ConsoleKind;
  viewers: number;
  onConnect: (kind: ConsoleKind) => void;
}

export function ConsoleToolbar({ status: consoleStatus, kind, viewers, onConnect }: ConsoleToolbarProps) {
  const { t } = useTranslation('ct');

  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          permission={CT_PERM.CONSOLE}
          disabled={consoleStatus === 'connecting' || consoleStatus === 'connected'}
          onClick={() => onConnect('tty')}
        >
          <IconPlugConnected className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('console.title')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          permission={CT_PERM.CONSOLE}
          disabled={consoleStatus === 'connecting' || consoleStatus === 'connected'}
          onClick={() => onConnect('exec')}
        >
          <IconTerminal2 className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {t('console.shell')}
        </Button>
        {(consoleStatus === 'disconnected' || consoleStatus === 'error') && (
          <Button variant="outline" size="sm" onClick={() => onConnect(kind)}>
            <IconRefresh className="mr-1.5 h-4 w-4" aria-hidden="true" />
            {t('actions.refresh')}
          </Button>
        )}
      </div>

      <div className="flex items-center gap-2">
        {consoleStatus === 'connecting' && <Spinner className="h-4 w-4" />}
        {consoleStatus === 'connected' && <Badge variant="success">{t('console.connected')}</Badge>}
        {consoleStatus === 'disconnected' && <Badge variant="secondary">{t('console.disconnected')}</Badge>}
        {/*
          Other viewers are shown because a console is shared: two operators
          typing into the same root shell at once is worth knowing about
          before it happens rather than after.
        */}
        {viewers > 1 && (
          <Badge variant="outline">
            <IconUsers className="mr-1 h-3 w-3" aria-hidden="true" />
            {viewers}
          </Badge>
        )}
      </div>
    </div>
  );
}
