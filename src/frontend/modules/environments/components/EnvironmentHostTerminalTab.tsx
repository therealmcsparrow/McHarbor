// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlugConnected, IconPlugX, IconShieldLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';

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

type EnvironmentHostTerminalTabProps = {
  envId: string;
};

export function EnvironmentHostTerminalTab({ envId }: EnvironmentHostTerminalTabProps) {
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
      // ignore
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
      const term = new Terminal({
        cursorBlink: true,
        fontFamily: 'JetBrains Mono, Fira Code, monospace',
        fontSize: 14,
        convertEol: true,
        theme: {
          background: '#0a0a0a',
          foreground: '#e5e5e5',
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
      const ws = new WebSocket(`${protocol}//${window.location.host}/api/system/host-terminal/ws?${params}`);
      ws.binaryType = 'arraybuffer';
      wsRef.current = ws;

      ws.onopen = () => {
        setConnecting(false);
        setConnected(true);
      };
      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data));
        } else if (typeof event.data === 'string') {
          // Server may send JSON error frames; ignore everything except
          // the textual error payload when present.
          try {
            const payload = JSON.parse(event.data) as { type?: string; data?: string };
            if (payload.type === 'error' && payload.data) {
              term.write(`\r\n\x1b[31m${payload.data}\x1b[0m\r\n`);
            }
          } catch {
            term.write(event.data);
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

  useEffect(() => {
    return () => disposeAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconShieldLock className="size-5 text-muted-foreground" />
            {t('detail.hostTerminal.title')}
          </CardTitle>
          <CardDescription>{t('detail.hostTerminal.description')}</CardDescription>
        </div>
        <div className="flex items-center gap-2">
          {connected ? (
            <Badge variant="success">{t('detail.hostTerminal.connected')}</Badge>
          ) : connecting ? (
            <Badge variant="secondary">{t('detail.hostTerminal.connecting')}</Badge>
          ) : (
            <Badge variant="secondary">{t('detail.hostTerminal.disconnected')}</Badge>
          )}
          {connected ? (
            <Button variant="outline" size="sm" onClick={disconnect}>
              <IconPlugX className="size-4" />
              {t('detail.hostTerminal.disconnect')}
            </Button>
          ) : (
            <Button size="sm" onClick={() => void connect()} disabled={connecting}>
              {connecting ? <Spinner size="sm" /> : <IconPlugConnected className="size-4" />}
              {t('detail.hostTerminal.connect')}
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div className="mb-3 rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-800 dark:text-yellow-200">
          {t('detail.hostTerminal.warning')}
        </div>
        {error && (
          <div className="mb-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}
        <div className="min-h-[480px] overflow-hidden rounded-lg border border-border bg-card">
          <div ref={termRef} className="h-[480px] w-full p-2" />
        </div>
      </CardContent>
    </Card>
  );
}