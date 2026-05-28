// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from "react";
import { useEnvironmentStore } from "@resources/stores/environment";

type XTermInstance = import("@xterm/xterm").Terminal;
type FitAddonInstance = import("@xterm/addon-fit").FitAddon;

let terminalAssetsPromise: Promise<{
  Terminal: typeof import("@xterm/xterm").Terminal;
  FitAddon: typeof import("@xterm/addon-fit").FitAddon;
}> | null = null;

function loadTerminalAssets() {
  if (!terminalAssetsPromise) {
    terminalAssetsPromise = Promise.all([
      import("@xterm/xterm"),
      import("@xterm/addon-fit"),
      import("@xterm/xterm/css/xterm.css"),
    ]).then(([xtermModule, fitModule]) => ({
      Terminal: xtermModule.Terminal,
      FitAddon: fitModule.FitAddon,
    }));
  }

  return terminalAssetsPromise;
}

export function useSystemTerminal() {
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
    if (!termRef.current) return;
    disconnect();
    const attemptId = connectAttemptRef.current;

    const { Terminal, FitAddon } = await loadTerminalAssets();
    if (attemptId !== connectAttemptRef.current || !termRef.current) {
      return;
    }

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "JetBrains Mono, Fira Code, monospace",
      fontSize: 14,
      theme: {
        background: "#0a0a0a",
        foreground: "#e4e4e7",
        cursor: "#e4e4e7",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(termRef.current);
    fit.fit();

    xtermRef.current = term;
    fitRef.current = fit;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const params = new URLSearchParams();
    if (envId) params.set("env", envId);
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const ws = new WebSocket(
      `${protocol}//${window.location.host}/api/system/os-terminal/ws${suffix}`,
    );
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      term.focus();
    };

    ws.onmessage = (event) => {
      term.write(
        typeof event.data === "string"
          ? event.data
          : new Uint8Array(event.data as ArrayBuffer),
      );
    };

    ws.onclose = () => {
      term.writeln("\r\n\x1b[31m--- Connection closed ---\x1b[0m");
      setConnected(false);
    };

    ws.onerror = () => {
      term.writeln("\r\n\x1b[31m--- Connection error ---\x1b[0m");
      setConnected(false);
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "input", data }));
      }
    });

    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", cols, rows }));
      }
    });
  }, [disconnect, envId]);

  useEffect(() => {
    const handleResize = () => fitRef.current?.fit();
    window.addEventListener("resize", handleResize);
    return () => {
      window.removeEventListener("resize", handleResize);
      disconnect();
    };
  }, [disconnect]);

  return { termRef, connected, connect, disconnect };
}
