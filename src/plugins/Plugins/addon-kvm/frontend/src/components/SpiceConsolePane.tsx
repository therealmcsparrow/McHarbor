// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useSpiceConsole } from '../hooks/useSpiceConsole';
import type { VM } from '../types';
import { ConsoleConnectButtons, ConsoleErrorLine, ConsoleStatusBadge, ConsoleViewersBadge } from './ConsoleControls';
import { ConsoleToolbar, type ConsoleDisplay, type ConsoleToolbarSlots } from './ConsoleToolbar';
import { ConsoleResizeHint } from './ConsoleResizeHint';
import { ConsoleScreen } from './ConsoleScreen';
import { spiceEvents } from '../spice-keys';
import { ConsoleKeysMenu } from './ConsoleKeysMenu';

/** The guest's display over SPICE, drawn by spice-html5. */
export function SpiceConsolePane({
  vm,
  slots,
  display,
}: {
  vm: VM;
  slots: ConsoleToolbarSlots;
  display: ConsoleDisplay;
}) {
  const { t } = useTranslation('vm');
  const {
    status: state,
    detail,
    viewers,
    agent,
    screenRef,
    viewportRef,
    connect,
    disconnect,
    sendKeys,
  } = useSpiceConsole(vm, display.mode, display.fullscreen);
  const live = state === 'connected' || state === 'connecting';

  return (
    <>
      <ConsoleToolbar slots={slots}>
        <ConsoleStatusBadge state={state} />
        <ConsoleViewersBadge viewers={viewers} />
        <ConsoleConnectButtons state={state} live={live} onConnect={() => void connect()} onDisconnect={disconnect} />
        <ConsoleKeysMenu disabled={state !== 'connected'} onSend={(strokes) => sendKeys(spiceEvents(strokes))} />
      </ConsoleToolbar>
      <ConsoleErrorLine state={state} detail={detail} />
      {state === 'connected' && (
        <ConsoleResizeHint
          kind="spice"
          mode={display.mode}
          adapter={vm.displayAdapter}
          hostResize={display.hostResize}
          guestUnanswered={agent === 'absent'}
        />
      )}
      {/* The picture is fitted with a transform, so the box clips rather than scrolls. */}
      <ConsoleScreen boxRef={viewportRef} className="mc-glass flex min-h-0 flex-1 overflow-hidden rounded-lg">
        <div
          ref={screenRef}
          className="h-full w-full"
          role="application"
          aria-label={t('console.kind.spice')}
          // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- the remote display captures the keyboard while focused, so it must take a tab stop
          tabIndex={0}
        />
      </ConsoleScreen>
    </>
  );
}
