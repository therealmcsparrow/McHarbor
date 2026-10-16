// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { useConsoleTicket } from './useVMDevices';
import { consoleSocketUrl } from '../console-url';
import { SERIAL_BACKGROUND, SERIAL_FOREGROUND } from '../serialTheme';
import type { ConsoleStatus } from '../console-sessions';
import type { VM } from '../types';

/**
 * The guest's first serial port in a terminal.
 *
 * A resize is sent as a JSON control frame, which the gateway turns into an
 * `stty` inside the guest through the guest agent. Without one the guest keeps
 * the size it booted with, which is why the console says so.
 */
export function useSerialConsole(vm: VM) {
  const [state, setStatus] = useState<ConsoleStatus>('idle');
  const [detail, setDetail] = useState('');
  const [viewers, setViewers] = useState(0);
  const screenRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const ticket = useConsoleTicket();

  const release = useCallback(() => {
    socketRef.current?.close();
    socketRef.current = null;
    terminalRef.current?.dispose();
    terminalRef.current = null;
    fitRef.current = null;
  }, []);

  const disconnect = useCallback(() => {
    release();
    setStatus('idle');
    setViewers(0);
  }, [release]);

  const connect = useCallback(async () => {
    release();
    setStatus('connecting');
    setDetail('');

    let issued;
    try {
      issued = await ticket.mutateAsync({ environmentId: vm.environmentId, guestId: vm.id, kind: 'serial' });
    } catch (error) {
      setStatus('error');
      setDetail(error instanceof Error ? error.message : String(error));
      return;
    }
    if (!screenRef.current) return;
    setViewers(issued.viewers);

    const terminal = new Terminal({
      convertEol: true,
      fontSize: 13,
      theme: { background: SERIAL_BACKGROUND, foreground: SERIAL_FOREGROUND },
    });
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(screenRef.current);
    fit.fit();
    terminalRef.current = terminal;
    fitRef.current = fit;

    const socket = new WebSocket(consoleSocketUrl(issued.ticket));
    socket.binaryType = 'arraybuffer';
    socketRef.current = socket;
    const sendSize = (cols: number, rows: number) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: 'resize', cols, rows }));
    };

    socket.onopen = () => {
      setStatus('connected');
      sendSize(terminal.cols, terminal.rows);
    };
    socket.onmessage = (evt) => {
      terminal.write(typeof evt.data === 'string' ? evt.data : new Uint8Array(evt.data));
    };
    socket.onclose = (evt) => {
      if (socketRef.current !== socket) return;
      setStatus('disconnected');
      if (evt.reason) setDetail(evt.reason);
    };
    socket.onerror = () => {
      if (socketRef.current === socket) setStatus('error');
    };
    terminal.onData((data) => {
      if (socket.readyState === WebSocket.OPEN) socket.send(data);
    });
    terminal.onResize(({ cols, rows }) => sendSize(cols, rows));
  }, [release, ticket, vm.environmentId, vm.id]);

  const refit = useCallback(() => fitRef.current?.fit(), []);

  useEffect(() => release, [release]);

  return { status: state, detail, viewers, screenRef, connect, disconnect, refit };
}
