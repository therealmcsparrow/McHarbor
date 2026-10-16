// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { useConsoleTicket } from '../hooks/useVMs';
import { useVncResize } from '../hooks/useVncResize';
import {
  attachConsole,
  closeConsole,
  consoleRFB,
  consoleSnapshot,
  detachConsole,
  failConsole,
  isConsoleLive,
  startConsole,
  stopConsole,
  subscribeConsole,
} from '../console-sessions';
import { consoleSocketUrl } from '../console-url';
import type { VM } from '../types';
import { sendVncStrokes } from '../vnc-keys';
import { ConsoleKeysMenu } from './ConsoleKeysMenu';
import { ConsoleMenu } from './VMConsoleMenu';
import { ConsoleConnectButtons, ConsoleErrorLine, ConsoleStatusBadge, ConsoleViewersBadge } from './ConsoleControls';
import { ConsoleToolbar, type ConsoleDisplay, type ConsoleToolbarSlots } from './ConsoleToolbar';
import { ConsoleResizeHint } from './ConsoleResizeHint';
import { ConsoleScreen } from './ConsoleScreen';

/**
 * The guest's graphical console over VNC.
 *
 * A short-lived, single-use ticket is requested over the ordinary
 * authenticated API, then the WebSocket is opened with that ticket in the path.
 * The session token is never in the URL: a URL lands in proxy logs and browser
 * history, and a leaked session token would be far worse than a leaked
 * 30-second ticket for one console.
 */
export function VncConsolePane({
  vm,
  slots,
  display,
}: {
  vm: VM;
  slots: ConsoleToolbarSlots;
  display: ConsoleDisplay;
}) {
  const { t } = useTranslation('vm');
  const snapshot = useSyncExternalStore(
    useCallback((listener: () => void) => subscribeConsole(vm.id, listener), [vm.id]),
    useCallback(() => consoleSnapshot(vm.id), [vm.id]),
  );
  const { status: state, detail, viewers } = snapshot;
  const screenRef = useRef<HTMLDivElement | null>(null);
  const ticket = useConsoleTicket();

  const guestUnanswered = useVncResize(vm.id, screenRef, state, display.mode, display.fullscreen);

  // The canvas lives outside this component so switching tab or page leaves
  // the session up. Mounting re-parents it; unmounting only detaches it.
  useEffect(() => {
    const screenEl = screenRef.current;
    if (!screenEl) return;
    attachConsole(vm.id, screenEl);
    return () => detachConsole(vm.id);
  }, [vm.id]);

  const connect = useCallback(async () => {
    await closeConsole(vm.id);
    let issued;
    try {
      issued = await ticket.mutateAsync({ environmentId: vm.environmentId, guestId: vm.id, kind: 'vnc' });
    } catch (error) {
      failConsole(vm.id, error instanceof Error ? error.message : String(error));
      return;
    }
    startConsole(vm.id, consoleSocketUrl(issued.ticket), issued.viewers);
  }, [ticket, vm.environmentId, vm.id]);

  return (
    <>
      <ConsoleToolbar slots={slots}>
        <ConsoleStatusBadge state={state} />
        <ConsoleViewersBadge viewers={viewers} />
        <ConsoleConnectButtons
          state={state}
          live={isConsoleLive(vm.id)}
          onConnect={() => void connect()}
          onDisconnect={() => stopConsole(vm.id)}
        />
        <ConsoleKeysMenu
          disabled={state !== 'connected'}
          onSend={(strokes) => sendVncStrokes(consoleRFB(vm.id), strokes)}
        />
        {state === 'connected' && <ConsoleMenu vmId={vm.id} vmName={vm.name} environmentId={vm.environmentId} />}
      </ConsoleToolbar>
      <ConsoleErrorLine state={state} detail={detail} />
      {state === 'connected' && (
        <ConsoleResizeHint
          kind="vnc"
          mode={display.mode}
          adapter={vm.displayAdapter}
          hostResize={display.hostResize}
          guestUnanswered={guestUnanswered}
        />
      )}
      <ConsoleScreen className="mc-glass flex min-h-0 flex-1 overflow-hidden rounded-lg">
        <div
          ref={screenRef}
          className="h-full w-full"
          // The console owns the keyboard while focused, so it is reachable
          // and labelled rather than a bare div.
          role="application"
          aria-label={t('console.title')}
          // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- the remote display captures the keyboard while focused, so it must take a tab stop
          tabIndex={0}
        />
      </ConsoleScreen>
    </>
  );
}
