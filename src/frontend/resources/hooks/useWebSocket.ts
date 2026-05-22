// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useCallback, useState } from 'react';

type UseWebSocketOptions = {
  url: string;
  onMessage?: (data: string) => void;
  onOpen?: () => void;
  onClose?: () => void;
  onError?: (error: Event) => void;
  enabled?: boolean;
};

export function useWebSocket({
  url,
  onMessage,
  onOpen,
  onClose,
  onError,
  enabled = true,
}: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null);
  const [readyState, setReadyState] = useState<number>(WebSocket.CLOSED);

  const connect = useCallback(() => {
    if (!enabled) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = url.startsWith('ws') ? url : `${protocol}//${window.location.host}${url}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setReadyState(WebSocket.OPEN);
      onOpen?.();
    };

    ws.onmessage = (event) => {
      onMessage?.(event.data);
    };

    ws.onerror = (event) => {
      onError?.(event);
    };

    ws.onclose = () => {
      setReadyState(WebSocket.CLOSED);
      onClose?.();
    };
  }, [url, onMessage, onOpen, onClose, onError, enabled]);

  useEffect(() => {
    connect();
    return () => {
      wsRef.current?.close();
    };
  }, [connect]);

  const send = useCallback((data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(data);
    }
  }, []);

  return {
    send,
    readyState,
    close: () => wsRef.current?.close(),
  };
}
