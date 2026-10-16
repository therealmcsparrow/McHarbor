// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconInfoCircle } from '@tabler/icons-react';
import { useSerialConsole } from '../hooks/useSerialConsole';
import type { VM } from '../types';
import { ConsoleConnectButtons, ConsoleErrorLine, ConsoleStatusBadge, ConsoleViewersBadge } from './ConsoleControls';
import { ConsoleToolbar, type ConsoleToolbarSlots } from './ConsoleToolbar';
import { ConsoleScreen } from './ConsoleScreen';

/** The guest's first serial port, as a terminal. */
export function SerialConsolePane({ vm, slots }: { vm: VM; slots: ConsoleToolbarSlots }) {
  const { t } = useTranslation('vm');
  const { status: state, detail, viewers, screenRef, connect, disconnect, refit } = useSerialConsole(vm);
  const live = state === 'connected' || state === 'connecting';

  useEffect(() => {
    const screenEl = screenRef.current;
    if (!screenEl || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => refit());
    observer.observe(screenEl);
    return () => observer.disconnect();
  }, [refit, screenRef]);

  return (
    <>
      <ConsoleToolbar slots={slots}>
        <ConsoleStatusBadge state={state} />
        <ConsoleViewersBadge viewers={viewers} />
        <ConsoleConnectButtons state={state} live={live} onConnect={() => void connect()} onDisconnect={disconnect} />
      </ConsoleToolbar>
      {state === 'connected' && vm.guestAgentState !== 'connected' && (
        <output className="mb-3 flex items-center gap-2 text-xs text-muted-foreground">
          <IconInfoCircle className="size-3.5 shrink-0" aria-hidden="true" />
          {t('console.serial.resizeNeedsAgent')}
        </output>
      )}
      <ConsoleErrorLine state={state} detail={detail} />
      <ConsoleScreen className="mc-glass flex min-h-[300px] flex-1 overflow-hidden rounded-lg p-2">
        <div ref={screenRef} className="h-full w-full" role="application" aria-label={t('console.kind.serial')} />
      </ConsoleScreen>
    </>
  );
}
