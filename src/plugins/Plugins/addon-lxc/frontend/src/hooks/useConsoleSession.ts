// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { useConsoleTicket } from './useContainers';
import { isLive, type ConsoleKind, type Container } from '../types';
import { TERMINAL_BACKGROUND, TERMINAL_FOREGROUND } from '../terminalTheme';

/**
 * # How the connection is made
 *
 * A short-lived, single-use ticket is requested over the ordinary authenticated
 * API, then the WebSocket is opened with that ticket in the path. The session
 * token is never in the URL: a URL lands in proxy logs and browser history, and
 * a leaked session token would be far worse than a leaked 30-second ticket for
 * one console.
 *
 * The socket is opened fresh on every connect for the same reason the ticket is
 * single-use — there is nothing to reuse, and reconnecting means asking again.
 */

export type ConsoleStatus = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error';

export function useConsoleSession(container: Container) {
  const [consoleStatus, setStatus] = useState<ConsoleStatus>('idle');
  const [detail, setDetail] = useState('');
  const [viewers, setViewers] = useState(0);
  const [kind, setKind] = useState<ConsoleKind>('tty');

  const screenRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);

  const ticket = useConsoleTicket();

  /** Tear everything down. Safe to call repeatedly. */
  const disconnect = useCallback(() => {
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    if (terminalRef.current) {
      terminalRef.current.dispose();
      terminalRef.current = null;
      fitRef.current = null;
    }
  }, []);

  const connect = useCallback(
    async (target: ConsoleKind) => {
      disconnect();
      setKind(target);
      setStatus('connecting');
      setDetail('');

      let issued;
      try {
        issued = await ticket.mutateAsync({
          environmentId: container.environmentId,
          containerId: container.id,
          kind: target,
        });
      } catch (error) {
        setStatus('error');
        setDetail(error instanceof Error ? error.message : String(error));
        return;
      }

      setViewers(issued.viewers);

      const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const url = `${scheme}://${window.location.host}/api/compute/console/${issued.ticket}`;

      if (!screenRef.current) return;
      const terminal = new Terminal({
        convertEol: true,
        fontSize: 13,
        // The host's own theme tokens are not reachable from a canvas, so the
        // terminal takes a dark palette that reads correctly in both themes.
        theme: { background: TERMINAL_BACKGROUND, foreground: TERMINAL_FOREGROUND },
      });
      const fit = new FitAddon();
      terminal.loadAddon(fit);
      terminal.open(screenRef.current);
      fit.fit();
      terminalRef.current = terminal;
      fitRef.current = fit;

      const socket = new WebSocket(url);
      socket.binaryType = 'arraybuffer';
      socketRef.current = socket;

      socket.onopen = () => {
        setStatus('connected');
        // Tell the far end the size, so the container's own line discipline wraps
        // where the operator sees it wrap.
        socket.send(JSON.stringify({ type: 'resize', cols: terminal.cols, rows: terminal.rows }));
      };
      socket.onmessage = (ev) => {
        if (typeof ev.data === 'string') {
          terminal.write(ev.data);
          return;
        }
        terminal.write(new Uint8Array(ev.data));
      };
      socket.onclose = (ev) => {
        setStatus('disconnected');
        if (ev.reason) setDetail(ev.reason);
      };
      socket.onerror = () => setStatus('error');

      terminal.onData((data) => {
        if (socket.readyState === WebSocket.OPEN) socket.send(data);
      });
      terminal.onResize(({ cols, rows }) => {
        if (socket.readyState === WebSocket.OPEN) {
          socket.send(JSON.stringify({ type: 'resize', cols, rows }));
        }
      });
    },
    [container.environmentId, container.id, disconnect, ticket],
  );

  // Disconnect when the component goes and when the container stops — either
  // invalidates the session.
  useEffect(() => () => disconnect(), [disconnect]);

  const live = isLive(container);
  if (!live && consoleStatus !== 'idle') setStatus('idle');

  useEffect(() => {
    if (!live) disconnect();
  }, [live, disconnect]);

  // Keep the terminal fitted to its container.
  useEffect(() => {
    const onResize = () => fitRef.current?.fit();
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);

  return { status: consoleStatus, detail, viewers, kind, screenRef, connect };
}
