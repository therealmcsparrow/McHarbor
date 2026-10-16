// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentUploadActive } from '@resources/stores/upload-activity';
import { useCurrentEnvironmentActivitySettings } from './useCurrentEnvironmentActivitySettings';

type DockerEvent = {
  type: string;
  action: string;
  actor: { id: string; attributes?: Record<string, string> };
  time: number;
  status?: string;
};

/**
 * Connects to the Docker events SSE stream and invalidates
 * TanStack Query caches when relevant events occur.
 * Mount once (e.g. in AppLayout) — no per-page wiring needed.
 */
export function useDockerEvents() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { currentEnvironment, trackContainerEventsEnabled } = useCurrentEnvironmentActivitySettings();
  const uploadActive = useEnvironmentUploadActive(envId);
  const sourceRef = useRef<EventSource | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const invalidate = useCallback(
    (evt: DockerEvent) => {
      switch (evt.type) {
        case 'container':
          // Only invalidate list + inspect queries — NOT container-processes,
          // container-logs, container-stats etc. which have their own polling.
          void queryClient.invalidateQueries({ queryKey: ['containers'], exact: false });
          void queryClient.invalidateQueries({ queryKey: ['dashboard-stats'] });
          void queryClient.invalidateQueries({ queryKey: ['events'] });
          break;
        case 'image':
          void queryClient.invalidateQueries({ queryKey: ['images'] });
          break;
        case 'volume':
          void queryClient.invalidateQueries({ queryKey: ['volumes'] });
          break;
        case 'network':
          void queryClient.invalidateQueries({ queryKey: ['networks'] });
          break;
      }
    },
    [queryClient],
  );

  const connectRef = useRef<() => void>(() => undefined);
  const connect = useCallback(() => {
    sourceRef.current?.close();
    if (reconnectTimer.current) {
      clearTimeout(reconnectTimer.current);
      reconnectTimer.current = null;
    }
    if (uploadActive || (currentEnvironment && !trackContainerEventsEnabled)) {
      return;
    }

    const params = new URLSearchParams();
    if (envId) params.set('env', envId);
    const url = `/api/events/stream${params.toString() ? `?${params}` : ''}`;

    const es = new EventSource(url);
    sourceRef.current = es;

    // Docker events arrive as named events whose type matches event.Type
    const handler = (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data) as DockerEvent;
        invalidate(data);
      } catch {
        // ignore malformed payloads
      }
    };

    es.addEventListener('container', handler);
    es.addEventListener('image', handler);
    es.addEventListener('volume', handler);
    es.addEventListener('network', handler);

    es.onerror = () => {
      es.close();
      // Reconnect after 5 seconds
      reconnectTimer.current = setTimeout(() => connectRef.current(), 5000);
    };
  }, [currentEnvironment, envId, invalidate, trackContainerEventsEnabled, uploadActive]);

  useEffect(() => {
    connectRef.current = connect;
    connect();
    return () => {
      sourceRef.current?.close();
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
      }
    };
  }, [connect]);
}
