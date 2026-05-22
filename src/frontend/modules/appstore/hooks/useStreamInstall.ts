// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { createClientId } from '@resources/utils/id';
import type { InstallEvent, PortMapping, VolumeMount } from '../types';
import type { LogEntry } from '../components/InstallProgress';

interface InstallPayload {
  slug: string;
  name: string;
  environmentId: string;
  ports?: PortMapping[];
  volumes?: VolumeMount[];
  envVars?: Record<string, string>;
}

function createLogEntry(message: string, phase?: InstallEvent['phase']): LogEntry {
  return {
    id: createClientId(),
    message,
    phase,
  };
}

export function useStreamInstall() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  const [installing, setInstalling] = useState(false);
  const [progress, setProgress] = useState<InstallEvent | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const reset = useCallback(() => {
    setInstalling(false);
    setProgress(null);
    setLogs([]);
  }, []);

  const startInstall = useCallback(
    (payload: InstallPayload) => {
      setInstalling(true);
      setProgress({ step: 0, total: 5, message: t('appStore.startingInstall'), status: 'progress' });
      setLogs([createLogEntry(t('appStore.startingInstall'))]);

      const controller = new AbortController();
      abortRef.current = controller;

      fetch('/api/app-store/install/stream', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        signal: controller.signal,
      })
        .then((res) => {
          if (!res.ok || !res.body) {
            setProgress({ step: 0, total: 5, message: t('appStore.connectFailed'), status: 'error' });
            return;
          }
          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';

          const read = (): Promise<void> =>
            reader.read().then(({ done, value }) => {
              if (done) return;
              buffer += decoder.decode(value, { stream: true });
              const lines = buffer.split('\n');
              buffer = lines.pop() ?? '';
              for (const line of lines) {
                if (line.startsWith('data: ')) {
                  try {
                    const event = JSON.parse(line.slice(6)) as InstallEvent;
                    setProgress(event);
                    if (event.message) {
                      setLogs((prev) => [...prev, createLogEntry(event.message, event.phase)]);
                    }
                    if (event.status === 'done' || event.status === 'error') {
                      if (event.status === 'done') {
                        queryClient.invalidateQueries({ queryKey: ['app-store'] });
                        queryClient.invalidateQueries({ queryKey: ['stacks'] });
                      }
                      return;
                    }
                  } catch { /* skip malformed lines */ }
                }
              }
              return read();
            });
          return read();
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === 'AbortError') return;
          setProgress({ step: 0, total: 5, message: t('appStore.networkError'), status: 'error' });
        });
    },
    [queryClient, t]
  );

  return { installing, progress, logs, startInstall, abort, reset };
}

