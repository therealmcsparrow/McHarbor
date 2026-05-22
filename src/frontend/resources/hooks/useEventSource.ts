// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useCallback } from 'react';

type UseEventSourceOptions = {
  url: string;
  onMessage: (data: string) => void;
  onError?: (error: Event) => void;
  enabled?: boolean;
};

export function useEventSource({
  url,
  onMessage,
  onError,
  enabled = true,
}: UseEventSourceOptions) {
  const sourceRef = useRef<EventSource | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    if (!enabled) return;

    const eventSource = new EventSource(url);
    sourceRef.current = eventSource;

    eventSource.onmessage = (event) => {
      onMessage(event.data);
    };

    eventSource.onerror = (event) => {
      onError?.(event);
      // Auto-reconnect after 5 seconds
      eventSource.close();
      reconnectTimerRef.current = setTimeout(connect, 5000);
    };
  }, [url, onMessage, onError, enabled]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      sourceRef.current?.close();
    };
  }, [connect]);

  return {
    close: () => sourceRef.current?.close(),
  };
}
