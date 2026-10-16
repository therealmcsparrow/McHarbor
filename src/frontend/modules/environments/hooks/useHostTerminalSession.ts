// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The xterm + WebSocket session behind the environment host terminal tab.
// Extracted from EnvironmentHostTerminalTab.tsx unchanged: same lazy asset
// load, same frame handling, same teardown on unmount.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { resolveAnsiPalette, resolveSurfaceTokens } from '@resources/theme/resolveToken';

type TerminalInstance = import('@xterm/xterm').Terminal;
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

// HostTerminalOptions mirror the shell/user/working-directory controls on the
// tab. All three are optional; omitting them reproduces the behaviour the host
// terminal had before the controls existed.
export type HostTerminalOptions = {
  shell?: string;
  user?: string;
  workingDir?: string;
};

export function useHostTerminalSession(envId: string, options: HostTerminalOptions = {}) {
  // Read through a ref so `connect` never goes stale between renders and the
  // caller does not have to memoise the options object.
  const optionsRef = useRef(options);
  useEffect(() => {
    optionsRef.current = options;
  });
  const { t } = useTranslation('environments');
  const termRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<TerminalInstance | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddonInstance | null>(null);
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const disposeAll = () => {
    try {
      wsRef.current?.close();
    } catch {
      // Closing a socket that never opened can throw; teardown continues either way.
    }
    wsRef.current = null;
    xtermRef.current?.dispose();
    xtermRef.current = null;
    fitRef.current = null;
    setConnected(false);
  };

  const connect = async () => {
    if (!envId || !termRef.current) return;
    setError(null);
    setConnecting(true);
    disposeAll();
    try {
      const { Terminal, FitAddon } = await loadTerminalAssets();
      const surface = resolveSurfaceTokens();
      const term = new Terminal({
        cursorBlink: true,
        fontFamily: 'JetBrains Mono, Fira Code, monospace',
        fontSize: 14,
        convertEol: true,
        theme: {
          background: surface.background,
          foreground: surface.foreground,
          cursor: surface.cursor,
          cursorAccent: surface.background,
          selectionBackground: surface.selection,
          ...resolveAnsiPalette(),
        },
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(termRef.current);
      fit.fit();
      xtermRef.current = term;
      fitRef.current = fit;

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const params = new URLSearchParams({
        env: envId,
        cols: String(term.cols),
        rows: String(term.rows),
      });
      const { shell, user, workingDir } = optionsRef.current;
      if (shell) params.set('shell', shell);
      if (user) params.set('user', user);
      if (workingDir) params.set('workingDir', workingDir);
      const ws = new WebSocket(`${protocol}//${window.location.host}/api/system/host-terminal/ws?${params}`);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        setConnecting(false);
        setConnected(true);
        // Without this the session connects and then ignores every keystroke.
        // The Connect button is disabled the moment it is clicked, which makes
        // the browser drop focus to <body>, and nothing else ever hands it to
        // xterm's hidden textarea. useXTerm - behind the container and sidebar
        // terminals, which is why those two were fine - has always done this.
        term.focus();
      };
      ws.onmessage = (msg) => {
        if (msg.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(msg.data));
        } else if (typeof msg.data === 'string') {
          // Server may send JSON error frames; ignore everything except
          // the textual error payload when present.
          try {
            const payload = JSON.parse(msg.data) as { type?: string; data?: string };
            if (payload.type === 'error' && payload.data) {
              term.write(`\r\n\x1b[31m${payload.data}\x1b[0m\r\n`);
            }
          } catch {
            term.write(msg.data);
          }
        }
      };
      ws.onerror = () => {
        setError(t('detail.hostTerminal.connectError'));
      };
      ws.onclose = () => {
        setConnecting(false);
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
    } catch (err) {
      setConnecting(false);
      setError((err as Error).message);
    }
  };

  const disconnect = () => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'end' }));
    }
    disposeAll();
  };

  // Exposed so the tab can put focus back after a fullscreen toggle, which
  // would otherwise leave it on the toggle button and reproduce the same
  // "connected but deaf" symptom in miniature.
  const focusTerminal = () => xtermRef.current?.focus();

  // Refit when the viewport changes - entering fullscreen included, which
  // dispatches a resize for exactly this. xterm's own onResize then forwards
  // the new geometry to the host, so the PTY follows the window instead of
  // staying at whatever size it was opened with.
  useEffect(() => {
    const handleResize = () => fitRef.current?.fit();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  useEffect(() => {
    return () => disposeAll();
  }, []);

  return { termRef, connected, connecting, error, connect, disconnect, focus: focusTerminal };
}
