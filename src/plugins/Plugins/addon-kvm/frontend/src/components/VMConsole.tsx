// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, CardContent } from '@resources/components/ui/Card';
import { TERMINAL_FULLSCREEN_CLASS, TerminalFullscreenButton } from '@resources/components/TerminalFullscreen';
import { useVMLifecycle, type LifecycleAction } from '../hooks/useVMs';
import { useConsoleFullscreen } from '../hooks/useConsoleFullscreen';
import { useConsoleDisplayMode } from '../hooks/useConsoleDisplayMode';
import { VM_FEATURE } from '../hooks/useQEMUCapability';
import { stopConsole } from '../console-sessions';
import type { ConsoleKind, VM } from '../types';
import { useHostFeature } from './HostFeatureGate';
import { GuestPowerControls } from './VMConsolePower';
import { ConsoleKindPicker } from './ConsoleKindPicker';
import { SpiceFileButton } from './SpiceFileButton';
import { ConsoleDisplayModePicker } from './ConsoleDisplayModePicker';
import { VncConsolePane } from './VncConsolePane';
import { SerialConsolePane } from './SerialConsolePane';
import { SpiceConsolePane } from './SpiceConsolePane';
import type { ConsoleDisplay, ConsoleToolbarSlots } from './ConsoleToolbar';

/**
 * The guest's console: VNC, its first serial port, or SPICE.
 *
 * Each kind opens its own gateway session. VNC outlives a tab switch because
 * its canvas is parked outside React; serial and SPICE close with their pane.
 */
export function VMConsole({ vm }: { vm: VM }) {
  const { t } = useTranslation('vm');
  const [pickedKind, setKind] = useState<ConsoleKind>('vnc');
  const { fullscreen, toggle: toggleFullscreen } = useConsoleFullscreen();
  const { mode, setMode } = useConsoleDisplayMode();
  const lifecycle = useVMLifecycle();
  const spice = useHostFeature(vm.environmentId, VM_FEATURE.SPICE);
  const runLifecycle = useCallback(
    (action: LifecycleAction) => lifecycle.mutate({ id: vm.id, action }),
    [lifecycle, vm.id],
  );

  const chooseKind = (next: ConsoleKind) => {
    // Leaving VNC ends its session: a parked session nobody can see still
    // holds one of the guest's viewer seats.
    if (kind === 'vnc' && next !== 'vnc') stopConsole(vm.id);
    setKind(next);
  };

  // A guest that is neither running nor paused has no console to hold open.
  const hasScreen = vm.state === 'running' || vm.state === 'paused';
  useEffect(() => {
    if (!hasScreen) stopConsole(vm.id);
  }, [hasScreen, vm.id]);

  // A host that lost SPICE (an agent downgrade, a re-probe) drops back to VNC
  // rather than leaving a pane that can never connect.
  const spiceSupported = spice.supported;
  const kind: ConsoleKind = pickedKind === 'spice' && spice.known && !spiceSupported ? 'vnc' : pickedKind;

  if (!hasScreen) {
    return (
      <div className="rounded-lg border border-border bg-card p-8 text-center">
        <p className="text-sm text-muted-foreground">{t('console.notRunning')}</p>
      </div>
    );
  }

  const display: ConsoleDisplay = {
    mode,
    fullscreen,
    // Absent, not false, from an agent that predates the key: claim nothing then.
    hostResize: spice.capability?.features?.[VM_FEATURE.DISPLAY_RESIZE],
  };

  const slots: ConsoleToolbarSlots = {
    leading: (
      <>
        <GuestPowerControls vm={vm} busy={lifecycle.isPending} onAction={runLifecycle} />
        <ConsoleKindPicker
          kind={kind}
          onChange={chooseKind}
          spiceSupported={spiceSupported}
          spiceReason={spice.reason}
        />
      </>
    ),
    trailing: (
      <>
        {kind !== 'serial' && <ConsoleDisplayModePicker mode={mode} onChange={setMode} />}
        <SpiceFileButton vm={vm} supported={spiceSupported} reason={spice.reason} />
        <TerminalFullscreenButton fullscreen={fullscreen} onToggle={toggleFullscreen} />
      </>
    ),
  };

  return (
    <div className={fullscreen ? TERMINAL_FULLSCREEN_CLASS : 'flex min-h-0 flex-1 flex-col'}>
      <Card className="flex min-h-0 flex-1 flex-col">
        <CardContent className="flex min-h-0 flex-1 flex-col pt-6">
          {kind === 'vnc' && <VncConsolePane vm={vm} slots={slots} display={display} />}
          {kind === 'serial' && <SerialConsolePane vm={vm} slots={slots} />}
          {kind === 'spice' && <SpiceConsolePane vm={vm} slots={slots} display={display} />}
        </CardContent>
      </Card>
    </div>
  );
}
