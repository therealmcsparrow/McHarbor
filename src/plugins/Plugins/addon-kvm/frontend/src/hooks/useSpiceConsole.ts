// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useConsoleTicket } from './useVMDevices';
import { consoleSocketUrl } from '../console-url';
import { SPICE_CHANNELS, openSpice, type SpiceHandle } from '../spice-session';
import { GUEST_ANSWER_GRACE_MS, type ConsoleDisplayMode } from '../console-resize';
import { watchSpiceResize } from '../spice-resize';
import type { ConsoleStatus } from '../console-sessions';
import type { SpiceKeyEvent } from '../spice-keys';
import type { VM } from '../types';

/** Whether the guest's spice-vdagent attached: it is the only thing that applies a resize. */
export type SpiceAgentState = 'unknown' | 'connected' | 'absent';

/**
 * A SPICE session into one guest, for as long as the screen is mounted.
 *
 * Unlike the VNC session it is not parked across tab switches: spice-html5
 * keeps its state on module globals and finds its screen by id, so a session
 * that outlives its element has nowhere to draw.
 */
export function useSpiceConsole(vm: VM, mode: ConsoleDisplayMode, fullscreen: boolean) {
  const [state, setStatus] = useState<ConsoleStatus>('idle');
  const [detail, setDetail] = useState('');
  const [viewers, setViewers] = useState(0);
  const [agent, setAgent] = useState<SpiceAgentState>('unknown');
  const screenRef = useRef<HTMLDivElement | null>(null);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const handleRef = useRef<SpiceHandle | null>(null);
  const modeRef = useRef(mode);
  const resizerRef = useRef<ReturnType<typeof watchSpiceResize> | null>(null);
  const generation = useRef(0);
  const ticket = useConsoleTicket();

  const disconnect = useCallback(() => {
    generation.current += 1;
    handleRef.current?.stop();
    handleRef.current = null;
    setStatus('idle');
    setViewers(0);
    setAgent('unknown');
  }, []);

  const connect = useCallback(async () => {
    disconnect();
    const mine = generation.current;
    setStatus('connecting');
    setDetail('');

    try {
      const issued = await ticket.mutateAsync({
        environmentId: vm.environmentId,
        guestId: vm.id,
        kind: 'spice',
        channels: SPICE_CHANNELS,
      });
      if (mine !== generation.current || !screenRef.current) return;
      setViewers(issued.viewers);
      const handle = await openSpice({
        screenEl: screenRef.current,
        urls: (issued.tickets ?? [issued.ticket]).map(consoleSocketUrl),
        onConnected: () => {
          if (mine === generation.current) setStatus('connected');
        },
        onError: (message) => {
          if (mine !== generation.current) return;
          setStatus('error');
          setDetail(message);
        },
        onAgent: () => {
          if (mine !== generation.current) return;
          setAgent('connected');
          // The first size goes as soon as there is someone to apply it.
          resizerRef.current?.applyNow();
        },
      });
      if (mine !== generation.current) {
        handle.stop();
        return;
      }
      handleRef.current = handle;
    } catch (error) {
      if (mine !== generation.current) return;
      setStatus('error');
      setDetail(error instanceof Error ? error.message : String(error));
    }
  }, [disconnect, ticket, vm.environmentId, vm.id]);

  useEffect(() => {
    const boxEl = viewportRef.current;
    const screenEl = screenRef.current;
    if (!boxEl || !screenEl) return;
    const resizer = watchSpiceResize({
      viewport: boxEl,
      screen: screenEl,
      mode: () => modeRef.current,
      target: () => handleRef.current,
    });
    resizerRef.current = resizer;
    return () => {
      resizer.dispose();
      resizerRef.current = null;
    };
  }, []);

  // Fullscreen and a mode change both move the box or what to do with it.
  useEffect(() => {
    modeRef.current = mode;
    resizerRef.current?.refresh();
  }, [mode, fullscreen]);

  // spice-html5 reports an agent only when one attaches, so silence after a
  // grace period is the only sign that nothing in the guest will resize it.
  useEffect(() => {
    if (state !== 'connected' || agent !== 'unknown') return;
    const timer = window.setTimeout(() => setAgent('absent'), GUEST_ANSWER_GRACE_MS);
    return () => window.clearTimeout(timer);
  }, [state, agent]);

  const sendKeys = useCallback((events: readonly SpiceKeyEvent[]) => handleRef.current?.sendKeys(events) ?? false, []);

  useEffect(
    () => () => {
      generation.current += 1;
      handleRef.current?.stop();
      handleRef.current = null;
    },
    [],
  );

  return { status: state, detail, viewers, agent, screenRef, viewportRef, connect, disconnect, sendKeys };
}
