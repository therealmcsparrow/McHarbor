// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlugConnected, IconPlugX, IconShieldLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import {
  SHELL_OPTIONS,
  buildUserOptions,
  USER_PRESET_DEFAULT,
  USER_PRESET_OTHER,
} from '@resources/components/TerminalShellOptions';
import {
  TERMINAL_FULLSCREEN_CLASS,
  TerminalFullscreenButton,
  useTerminalFullscreen,
} from '@resources/components/TerminalFullscreen';
import { PERM } from '@core/auth/permissionKeys';
import { useHostTerminalSession } from '../hooks/useHostTerminalSession';

type EnvironmentHostTerminalTabProps = {
  envId: string;
};

export function EnvironmentHostTerminalTab({ envId }: EnvironmentHostTerminalTabProps) {
  const { t } = useTranslation('environments');
  const [shell, setShell] = useState('');
  const [userPreset, setUserPreset] = useState(USER_PRESET_DEFAULT);
  const [customUser, setCustomUser] = useState('');
  const [workingDir, setWorkingDir] = useState('');
  const { fullscreen, toggle: toggleFullscreen } = useTerminalFullscreen();

  // The shell list is not probed the way the container one is: probing a host
  // costs a privileged container start (or an agent round trip) before the
  // operator has asked for anything. The backend falls back to a shell that
  // exists when the requested one does not, so an unavailable entry costs a
  // different shell, never a failed session.
  const userOptions = useMemo(
    () => buildUserOptions(t('detail.hostTerminal.userDefault'), t('detail.hostTerminal.userCustom')),
    [t],
  );

  // Empty string means "stay as the session's own account", which on a host
  // shell is root — the same shape the container surfaces use for "let the
  // image's USER apply".
  const resolvedUser =
    userPreset === USER_PRESET_OTHER ? customUser.trim() : userPreset === USER_PRESET_DEFAULT ? '' : userPreset;

  const {
    termRef,
    connected,
    connecting,
    error,
    connect,
    disconnect,
    focus: focusTerminal,
  } = useHostTerminalSession(envId, {
    shell,
    user: resolvedUser,
    workingDir: workingDir.trim(),
  });

  // Locked while a session is live: these apply when the shell starts, so
  // leaving them editable would suggest they take effect mid-session.
  const controlsLocked = connected || connecting;

  return (
    // The fullscreen class goes on a plain wrapper, never on the Card. Card
    // renders `mc-glass`, and a backdrop-filter creates a containing block for
    // fixed-position descendants — which is what stopped this overlay covering
    // the viewport while the container tab and sidebar page, both plain divs,
    // worked. Same shape as those two now.
    <div className={fullscreen ? TERMINAL_FULLSCREEN_CLASS : undefined}>
      <Card className={fullscreen ? 'min-h-0 flex-1' : undefined}>
        <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <IconShieldLock className="size-5 text-muted-foreground" />
              {t('detail.hostTerminal.title')}
            </CardTitle>
            <CardDescription>{t('detail.hostTerminal.description')}</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            {connected ? (
              <Badge variant="success">{t('detail.hostTerminal.connected')}</Badge>
            ) : connecting ? (
              <Badge variant="secondary">{t('detail.hostTerminal.connecting')}</Badge>
            ) : (
              <Badge variant="secondary">{t('detail.hostTerminal.disconnected')}</Badge>
            )}
          </div>
        </CardHeader>
        <CardContent className={fullscreen ? 'flex min-h-0 flex-1 flex-col' : undefined}>
          <div className="mb-3 flex flex-wrap items-center gap-3">
            <div className="w-32">
              <Select
                value={shell}
                onChange={setShell}
                options={SHELL_OPTIONS}
                ariaLabel={t('shellLabel', { ns: 'terminal' })}
                searchable={false}
                disabled={controlsLocked}
              />
            </div>
            <div className="w-44">
              <Select
                value={userPreset}
                onChange={setUserPreset}
                options={userOptions}
                ariaLabel={t('userLabel', { ns: 'terminal' })}
                searchable={false}
                disabled={controlsLocked}
              />
            </div>
            {userPreset === USER_PRESET_OTHER && (
              <Input
                value={customUser}
                onChange={(e) => setCustomUser(e.target.value)}
                placeholder={t('detail.hostTerminal.userPlaceholder')}
                aria-label={t('userLabel', { ns: 'terminal' })}
                className="w-44"
                disabled={controlsLocked}
              />
            )}
            <Input
              value={workingDir}
              onChange={(e) => setWorkingDir(e.target.value)}
              placeholder={t('workDirPlaceholder', { ns: 'terminal' })}
              aria-label={t('workDirLabel', { ns: 'terminal' })}
              className="w-40"
              disabled={controlsLocked}
            />
            {connected ? (
              <Button variant="outline" onClick={disconnect}>
                <IconPlugX className="size-4" />
                {t('detail.hostTerminal.disconnect')}
              </Button>
            ) : (
              <Button permission={PERM.TERMINAL_ACCESS} onClick={() => void connect()} disabled={connecting}>
                {connecting ? <Spinner size="sm" /> : <IconPlugConnected className="size-4" />}
                {t('detail.hostTerminal.connect')}
              </Button>
            )}
            <TerminalFullscreenButton
              fullscreen={fullscreen}
              onToggle={() => {
                toggleFullscreen();
                // Focus follows the terminal, not the button that resized it.
                window.requestAnimationFrame(focusTerminal);
              }}
            />
          </div>
          {/* In fullscreen every row belongs to the shell: the standing warning
              is already acknowledged by the time someone maximises. */}
          {!fullscreen && (
            <div className="mb-3 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">
              {t('detail.hostTerminal.warning')}
            </div>
          )}
          {error && (
            <div className="mb-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}
          <div
            className={
              fullscreen
                ? 'mc-glass flex min-h-0 flex-1 overflow-hidden rounded-lg'
                : 'mc-glass min-h-[480px] overflow-hidden rounded-lg'
            }
          >
            <div ref={termRef} className={fullscreen ? 'h-full w-full p-2' : 'h-[480px] w-full p-2'} />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
