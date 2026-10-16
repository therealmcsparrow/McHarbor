// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { useConsoleSession } from '../hooks/useConsoleSession';
import { ConsoleToolbar } from './ConsoleToolbar';
import { isLive, type Container } from '../types';
import { cssVars } from '@resources/utils/cssVars';
import { TERMINAL_BACKGROUND } from '../terminalTheme';

/**
 * The container console.
 *
 * Text only, unlike addon-kvm's: a system container has a pty and no
 * framebuffer. There is no VNC server inside a container to connect to, so
 * there is no graphical console to offer — and noVNC is not in this bundle at
 * all, which is most of why it is a third the size of the VM plugin's. The two
 * kinds are PID 1's own terminal (tty) and a new root shell (exec).
 */

export function ContainerConsole({ container }: { container: Container }) {
  const { t } = useTranslation('ct');
  const { status: consoleStatus, detail, viewers, kind, screenRef, connect } = useConsoleSession(container);

  if (!isLive(container)) {
    return (
      <div className="rounded-lg border border-border bg-card p-8 text-center">
        <p className="text-sm text-muted-foreground">{t('console.notRunning')}</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <ConsoleToolbar
        status={consoleStatus}
        kind={kind}
        viewers={viewers}
        onConnect={(target) => void connect(target)}
      />

      {/*
        Stated plainly rather than implied by the permission. A container
        console attaches to PID 1's pty as root — there is no login prompt in
        the way, and somebody who opened this tab out of curiosity should know
        that before they type.
      */}
      <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
        <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
        <p className="text-sm text-foreground">{t('console.rootShellWarning')}</p>
      </div>

      <p className="text-sm text-muted-foreground">{t('console.modeHint')}</p>

      {consoleStatus === 'error' && detail && (
        <p className="rounded-md bg-destructive/10 p-3 text-sm text-foreground">{detail}</p>
      )}
      {consoleStatus === 'disconnected' && detail && <p className="text-sm text-muted-foreground">{detail}</p>}

      <div
        ref={screenRef}
        className="min-h-[420px] w-full overflow-hidden rounded-lg border border-border bg-(--terminal-bg)"
        style={cssVars({ 'terminal-bg': TERMINAL_BACKGROUND })}
      />
    </div>
  );
}
