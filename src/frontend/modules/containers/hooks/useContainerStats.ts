// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCurrentEnvironmentActivitySettings } from '@resources/hooks/useCurrentEnvironmentActivitySettings';
import type { ContainerMetric } from '@core/types/docker';

const MAX_HISTORY = 60;
const RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 10;

export type UseContainerStatsResult = {
  current: ContainerMetric | null;
  history: ContainerMetric[];
  connected: boolean;
};

type StatsState = {
  key: string;
  current: ContainerMetric | null;
  history: ContainerMetric[];
  connected: boolean;
};

const EMPTY_STATE: StatsState = { key: '', current: null, history: [], connected: false };

export function useContainerStats(containerId: string, enabled = true): UseContainerStatsResult {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { collectContainerMetricsEnabled } = useCurrentEnvironmentActivitySettings();
  const shouldConnect = enabled && collectContainerMetricsEnabled;
  const key = `${envId ?? ''}|${containerId}`;
  const [state, setState] = useState<StatsState>(EMPTY_STATE);

  useEffect(() => {
    if (!shouldConnect || !containerId) return;

    let source: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let retries = 0;
    let disposed = false;

    const patch = (update: (prev: StatsState) => Partial<StatsState>) => {
      setState((prev) => {
        const base = prev.key === key ? prev : { ...EMPTY_STATE, key };
        return { ...base, ...update(base) };
      });
    };

    const connect = () => {
      if (disposed) return;
      const params = new URLSearchParams();
      if (envId) params.set('env', envId);
      const es = new EventSource(`/api/metrics/containers/${containerId}/stream?${params.toString()}`);
      source = es;

      es.addEventListener('connected', () => {
        if (source !== es) return;
        retries = 0;
        patch(() => ({ connected: true }));
      });

      es.addEventListener('stats', (message) => {
        if (source !== es) return;
        const metric: ContainerMetric = JSON.parse(message.data);
        patch((prev) => {
          const next = [...prev.history, metric];
          return { current: metric, history: next.length > MAX_HISTORY ? next.slice(-MAX_HISTORY) : next };
        });
      });

      es.onerror = () => {
        if (source !== es) return;
        patch(() => ({ connected: false }));
        es.close();
        source = null;
        if (retries < MAX_RECONNECT_ATTEMPTS) {
          retries++;
          timer = setTimeout(connect, RECONNECT_DELAY);
        }
      };
    };

    connect();
    return () => {
      disposed = true;
      if (timer) clearTimeout(timer);
      source?.close();
      source = null;
      setState(EMPTY_STATE);
    };
  }, [containerId, envId, key, shouldConnect]);

  const view = shouldConnect && state.key === key ? state : EMPTY_STATE;
  return { current: view.current, history: view.history, connected: view.connected };
}
