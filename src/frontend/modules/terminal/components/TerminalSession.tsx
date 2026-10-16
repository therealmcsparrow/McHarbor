// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { useXTerm } from '@resources/components/XTermPanel';

type TerminalSessionProps = {
  containerId: string;
  active: boolean;
  onDisconnect: () => void;
  shell?: string;
  user?: string;
  workingDir?: string;
};

export default function TerminalSession({
  containerId,
  active,
  onDisconnect,
  shell,
  user,
  workingDir,
}: TerminalSessionProps) {
  const { t } = useTranslation('terminal');
  const { termRef, connected } = useXTerm(containerId, {
    autoConnect: active,
    active,
    shell,
    user,
    workingDir,
  });

  return (
    <>
      <div ref={termRef} className="min-h-[400px] flex-1 rounded-lg border border-border bg-background p-1" />
      {connected && (
        <div className="flex justify-end">
          <Button variant="outline" onClick={onDisconnect}>
            {t('disconnect')}
          </Button>
        </div>
      )}
    </>
  );
}
