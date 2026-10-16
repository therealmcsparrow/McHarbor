// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { useXTerm } from '@resources/components/XTermPanel';

type TerminalPanelProps = {
  containerId: string;
};

export function TerminalPanel({ containerId }: TerminalPanelProps) {
  const { t } = useTranslation('containers');
  const { termRef, connected, connect } = useXTerm(containerId, { autoConnect: true });

  return (
    <div className="px-4 pb-4">
      <div className="mb-2 flex items-center gap-2">
        {connected ? (
          <span className="text-xs text-success">{t('terminal.connected')}</span>
        ) : (
          <Button size="sm" onClick={connect}>
            {t('actions.reconnect')}
          </Button>
        )}
      </div>
      <div ref={termRef} className="min-h-[400px] rounded-lg border border-border bg-background p-1" />
    </div>
  );
}
