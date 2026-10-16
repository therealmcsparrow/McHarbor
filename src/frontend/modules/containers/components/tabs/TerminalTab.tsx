// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBug, IconTerminal } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { useXTerm } from '@resources/components/XTermPanel';
import { USER_OPTIONS, USER_PRESET_DEFAULT, USER_PRESET_OTHER } from '@resources/components/TerminalShellOptions';
import { useApplicableShells } from '@resources/hooks/useContainerShells';
import {
  TERMINAL_FULLSCREEN_CLASS,
  TerminalFullscreenButton,
  useTerminalFullscreen,
} from '@resources/components/TerminalFullscreen';
import { DebugShellDialog } from '../DebugShellDialog';
import { useDebugShellAttach, type DebugShellSession } from '../../hooks/useDebugShell';
import { useContainerHostSelfGate } from '../../hooks/useHostSelfGate';

type TerminalTabProps = {
  containerId: string;
  isRunning: boolean;
  active: boolean;
};

export function TerminalTab({ containerId, isRunning, active }: TerminalTabProps) {
  const { t } = useTranslation('containers');
  const gate = useContainerHostSelfGate(containerId);
  const [shell, setShell] = useState('');
  const [userPreset, setUserPreset] = useState(USER_PRESET_DEFAULT);
  const [customUser, setCustomUser] = useState('');
  const [workingDir, setWorkingDir] = useState('');
  const { options: shellOptions } = useApplicableShells(containerId);
  const { fullscreen, toggle: toggleFullscreen } = useTerminalFullscreen();
  const [debugOpen, setDebugOpen] = useState(false);
  const [debugSession, setDebugSession] = useState<DebugShellSession | null>(null);
  const clearDebugSession = useCallback(() => setDebugSession(null), []);

  const { termRef, connected, connect, disconnect } = useXTerm(containerId, {
    active,
    shell,
    user: userPreset === USER_PRESET_OTHER ? customUser.trim() : userPreset === USER_PRESET_DEFAULT ? '' : userPreset,
    workingDir,
    debugSession: debugSession?.sessionId,
  });
  useDebugShellAttach(debugSession, connected, connect, clearDebugSession);

  if (!isRunning) {
    return (
      <div className="mc-glass flex h-64 items-center justify-center rounded-lg text-muted-foreground">
        <IconTerminal className="mr-2 h-5 w-5" />
        {t('terminal.mustBeRunning')}
      </div>
    );
  }

  return (
    <div className={fullscreen ? TERMINAL_FULLSCREEN_CLASS : 'flex min-h-0 flex-1 flex-col gap-3'}>
      <div className="flex shrink-0 flex-wrap items-center gap-3">
        <div className="w-32">
          <Select
            value={shell}
            onChange={setShell}
            options={shellOptions}
            ariaLabel={t('terminal.shellLabel')}
            searchable={false}
          />
        </div>
        <div className="w-44">
          <Select
            value={userPreset}
            onChange={setUserPreset}
            options={USER_OPTIONS}
            ariaLabel={t('terminal.userLabel')}
            searchable={false}
          />
        </div>
        {userPreset === USER_PRESET_OTHER && (
          <Input
            value={customUser}
            onChange={(e) => setCustomUser(e.target.value)}
            placeholder={t('terminal.userPlaceholder')}
            aria-label={t('terminal.userLabel')}
            className="w-44"
          />
        )}
        <Input
          value={workingDir}
          onChange={(e) => setWorkingDir(e.target.value)}
          placeholder={t('terminal.workDirPlaceholder')}
          aria-label={t('terminal.workDirLabel')}
          className="w-40"
        />
        <Button
          onClick={connect}
          permission={gate.exec([PERM.TERMINAL_ACCESS, PERM.CONTAINERS_MANAGE])}
          disabled={connected || !active}
        >
          <IconTerminal className="h-4 w-4" /> {t('actions.connect')}
        </Button>
        {connected && (
          <Button variant="outline" onClick={disconnect}>
            {t('actions.disconnect')}
          </Button>
        )}
        <Button
          variant="outline"
          permission={gate.exec(PERM.CONTAINERS_DEBUG_SHELL)}
          onClick={() => setDebugOpen(true)}
          disabled={connected || !active}
        >
          <IconBug className="h-4 w-4" /> {t('debugShell.button')}
        </Button>
        {connected && <span className="text-xs text-success">{t('terminal.connected')}</span>}
        {connected && debugSession && (
          <span className="text-xs text-warning">
            {t('debugShell.active', { path: debugSession.path, arch: debugSession.arch })}
          </span>
        )}
        <TerminalFullscreenButton fullscreen={fullscreen} onToggle={toggleFullscreen} />
      </div>
      <DebugShellDialog
        containerId={containerId}
        open={debugOpen}
        onOpenChange={setDebugOpen}
        onSuccess={setDebugSession}
      />

      <div ref={termRef} className="min-h-0 flex-1 rounded-lg border border-border bg-background p-1" />
    </div>
  );
}
