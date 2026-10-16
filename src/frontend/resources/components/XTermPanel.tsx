// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState, useCallback } from 'react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { resolveAnsiPalette, resolveSurfaceTokens } from '@resources/theme/resolveToken';

type UseXTermOptions = {
  /** Auto-connect on mount. Default: false */
  autoConnect?: boolean;
  /** Disconnect when false. Default: true */
  active?: boolean;
  /** Shell path (e.g. /bin/bash). Empty = auto-detect. */
  shell?: string;
  /** User identity (root / nobody / uid / uid:gid). Empty = image USER. */
  user?: string;
  /** Initial cwd inside the shell. Empty = shell's default. */
  workingDir?: string;
  debugSession?: string;
};

type XTermInstance = import('@xterm/xterm').Terminal;
type FitAddonInstance = import('@xterm/addon-fit').FitAddon;

let terminalAssetsPromise: Promise<{
  Terminal: typeof import('@xterm/xterm').Terminal;
  FitAddon: typeof import('@xterm/addon-fit').FitAddon;
}> | null = null;

function loadTerminalAssets() {
  if (!terminalAssetsPromise) {
    terminalAssetsPromise = Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/xterm/css/xterm.css'),
    ]).then(([xtermModule, fitModule]) => ({
      Terminal: xtermModule.Terminal,
      FitAddon: fitModule.FitAddon,
    }));
  }

  return terminalAssetsPromise;
}

/**
 * Hook that manages an xterm.js terminal with WebSocket connection to the backend.
 * Returns a ref to attach to a container div, plus connect/disconnect controls.
 *
 * Usage:
 * ```tsx
 * const { termRef, connected, connect, disconnect } = useXTerm(containerId);
 * return <div ref={termRef} className="..." />;
 * ```
 */
export function useXTerm(containerId: string, options?: UseXTermOptions) {
  const { autoConnect = false, active = true, shell, user, workingDir, debugSession } = options ?? {};
  const envId = useEnvironmentStore((s) => s.currentId);
  const [connected, setConnected] = useState(false);
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTermInstance | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddonInstance | null>(null);
  const connectAttemptRef = useRef(0);

  const disconnect = useCallback(() => {
    connectAttemptRef.current += 1;
    wsRef.current?.close();
    wsRef.current = null;
    xtermRef.current?.dispose();
    xtermRef.current = null;
    fitRef.current = null;
    setConnected(false);
  }, []);

  const connect = useCallback(async () => {
    if (!containerId || !termRef.current) return;
    disconnect();
    const attemptId = connectAttemptRef.current;

    const { Terminal, FitAddon } = await loadTerminalAssets();
    if (attemptId !== connectAttemptRef.current || !termRef.current) {
      return;
    }

    const surface = resolveSurfaceTokens();
    const ansi = resolveAnsiPalette();
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, Fira Code, monospace',
      fontSize: 14,
      theme: {
        background: surface.background,
        foreground: surface.foreground,
        cursor: surface.cursor,
        cursorAccent: surface.background,
        selectionBackground: surface.selection,
        ...ansi,
      },
    });

    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(termRef.current);
    fit.fit();

    xtermRef.current = term;
    fitRef.current = fit;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const params = new URLSearchParams({ container: containerId });
    if (shell && !debugSession) params.set('shell', shell);
    if (user) params.set('user', user);
    if (workingDir) params.set('workDir', workingDir);
    if (debugSession) params.set('debugSession', debugSession);
    if (envId) params.set('env', envId);
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/terminal/ws?${params}`);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      term.focus();
    };

    ws.onmessage = (e) => {
      term.write(typeof e.data === 'string' ? e.data : new Uint8Array(e.data as ArrayBuffer));
    };

    ws.onclose = () => {
      term.writeln('\r\n\x1b[31m--- Connection closed ---\x1b[0m');
      setConnected(false);
    };

    ws.onerror = () => {
      term.writeln('\r\n\x1b[31m--- Connection error ---\x1b[0m');
      setConnected(false);
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }));
      }
    });

    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });
  }, [containerId, envId, disconnect, shell, user, workingDir, debugSession]);

  // Disconnect when inactive
  useEffect(() => {
    if (!active && connected) {
      // eslint-disable-next-line react/set-state-in-effect -- tears down the external WebSocket and terminal, whose cleanup also resets the connected flag
      disconnect();
    }
  }, [active, connected, disconnect]);

  // Auto-connect
  useEffect(() => {
    if (!autoConnect) return;
    const timer = setTimeout(connect, 100);
    return () => {
      clearTimeout(timer);
      disconnect();
    };
  }, [autoConnect, connect, disconnect]);

  // Resize handler + cleanup on unmount
  useEffect(() => {
    const handleResize = () => fitRef.current?.fit();
    window.addEventListener('resize', handleResize);
    return () => {
      window.removeEventListener('resize', handleResize);
      disconnect();
    };
  }, [disconnect]);

  return { termRef, connected, connect, disconnect };
}
