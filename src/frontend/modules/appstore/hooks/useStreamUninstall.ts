// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { createClientId } from '@resources/utils/id';
import type { InstallEvent } from '../types';
import type { LogEntry } from '../components/InstallProgress';

function createLogEntry(message: string, phase?: InstallEvent['phase']): LogEntry {
  return {
    id: createClientId(),
    message,
    phase,
  };
}

export function useStreamUninstall() {
  const { t } = useTranslation('common');
  const queryClient = useQueryClient();
  const [uninstalling, setUninstalling] = useState(false);
  const [progress, setProgress] = useState<InstallEvent | null>(null);
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  const reset = useCallback(() => {
    setUninstalling(false);
    setProgress(null);
    setLogs([]);
  }, []);

  const startUninstall = useCallback(
    (installationId: string) => {
      setUninstalling(true);
      setProgress({ step: 0, total: 10, message: t('appStore.startingUninstall'), status: 'progress' });
      setLogs([createLogEntry(t('appStore.startingUninstall'))]);

      const controller = new AbortController();
      abortRef.current = controller;

      fetch(`/api/app-store/installations/${installationId}/uninstall/stream`, {
        method: 'POST',
        credentials: 'include',
        signal: controller.signal,
      })
        .then((res) => {
          if (!res.ok || !res.body) {
            const entry: InstallEvent = {
              step: 0,
              total: 10,
              message: t('appStore.uninstallConnectFailed'),
              status: 'error',
            };
            setUninstalling(false);
            setProgress(entry);
            setLogs((prev) => [...prev, createLogEntry(entry.message)]);
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
                    const eventItem = JSON.parse(line.slice(6)) as InstallEvent;
                    setProgress(eventItem);
                    if (eventItem.message) {
                      setLogs((prev) => [...prev, createLogEntry(eventItem.message, eventItem.phase)]);
                    }
                    if (eventItem.status === 'done' || eventItem.status === 'error') {
                      setUninstalling(false);
                      if (eventItem.status === 'done') {
                        void queryClient.invalidateQueries({ queryKey: ['app-store'] });
                        void queryClient.invalidateQueries({ queryKey: ['stacks'] });
                      }
                      return;
                    }
                  } catch {
                    // Ignore malformed SSE lines; the stream may continue with valid events.
                  }
                }
              }
              return read();
            });
          return read();
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === 'AbortError') return;
          const evt: InstallEvent = {
            step: 0,
            total: 10,
            message: t('appStore.uninstallNetworkError'),
            status: 'error',
          };
          setUninstalling(false);
          setProgress(evt);
          setLogs((prev) => [...prev, createLogEntry(evt.message)]);
        });
    },
    [queryClient, t],
  );

  return { uninstalling, progress, logs, startUninstall, abort, reset };
}
