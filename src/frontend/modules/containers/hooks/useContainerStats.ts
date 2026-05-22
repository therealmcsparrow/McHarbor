// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback, useRef, useEffect } from 'react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import type { ContainerMetric } from '@core/types/docker';

const MAX_HISTORY = 60;
const RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 10;

type UseContainerStatsResult = {
  current: ContainerMetric | null;
  history: ContainerMetric[];
  connected: boolean;
};

export function useContainerStats(containerId: string, enabled = true): UseContainerStatsResult {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { collectContainerMetricsEnabled } = useCurrentEnvironmentActivitySettings();
  const shouldConnect = enabled && collectContainerMetricsEnabled;
  const [current, setCurrent] = useState<ContainerMetric | null>(null);
  const [history, setHistory] = useState<ContainerMetric[]>([]);
  const [connected, setConnected] = useState(false);
  const sourceRef = useRef<EventSource | null>(null);
  const retriesRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const cleanup = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (sourceRef.current) {
      sourceRef.current.close();
      sourceRef.current = null;
    }
  }, []);

  const connect = useCallback(() => {
    if (!shouldConnect || !containerId) return;

    cleanup();

    const params = new URLSearchParams();
    if (envId) params.set('env', envId);
    const url = `/api/metrics/containers/${containerId}/stream?${params.toString()}`;

    const es = new EventSource(url);
    sourceRef.current = es;

    es.addEventListener('connected', () => {
      setConnected(true);
      retriesRef.current = 0;
    });

    es.addEventListener('stats', (event) => {
      const metric: ContainerMetric = JSON.parse(event.data);
      setCurrent(metric);
      setHistory((prev) => {
        const next = [...prev, metric];
        return next.length > MAX_HISTORY ? next.slice(-MAX_HISTORY) : next;
      });
    });

    es.addEventListener('error', () => {
      setConnected(false);
      es.close();
      sourceRef.current = null;

      if (retriesRef.current < MAX_RECONNECT_ATTEMPTS) {
        retriesRef.current++;
        timerRef.current = setTimeout(connect, RECONNECT_DELAY);
      }
    });

    es.onerror = () => {
      setConnected(false);
      es.close();
      sourceRef.current = null;

      if (retriesRef.current < MAX_RECONNECT_ATTEMPTS) {
        retriesRef.current++;
        timerRef.current = setTimeout(connect, RECONNECT_DELAY);
      }
    };
  }, [cleanup, containerId, envId, shouldConnect]);

  useEffect(() => {
    if (!shouldConnect) {
      cleanup();
      setConnected(false);
      setCurrent(null);
      setHistory([]);
      return cleanup;
    }

    connect();
    return cleanup;
  }, [cleanup, connect, shouldConnect]);

  return { current, history, connected };
}
