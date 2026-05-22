// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTerminal } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { useXTerm } from '@resources/components/XTermPanel';

type TerminalTabProps = {
  containerId: string;
  isRunning: boolean;
  active: boolean;
};

export function TerminalTab({ containerId, isRunning, active }: TerminalTabProps) {
  const { t } = useTranslation('containers');
  const { termRef, connected, connect, disconnect } = useXTerm(containerId, { active });

  if (!isRunning) {
    return (
      <div className="flex h-64 items-center justify-center rounded-lg border border-border bg-card text-muted-foreground">
        <IconTerminal className="mr-2 h-5 w-5" />
        {t('terminal.mustBeRunning')}
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <div className="flex shrink-0 items-center gap-3">
        <Button onClick={connect} disabled={connected || !active}>
          <IconTerminal className="h-4 w-4" /> {t('actions.connect')}
        </Button>
        {connected && (
          <Button variant="outline" onClick={disconnect}>
            {t('actions.disconnect')}
          </Button>
        )}
        {connected && (
          <span className="text-xs text-green-500">{t('terminal.connected')}</span>
        )}
      </div>

      <div
        ref={termRef}
        className="min-h-0 flex-1 rounded-lg border border-border bg-[#0a0a0a] p-1"
      />
    </div>
  );
}
