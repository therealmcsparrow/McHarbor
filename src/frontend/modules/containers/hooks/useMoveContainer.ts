// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import { createClientId } from '@resources/utils/id';
import type {
  MoveContainerEvent,
  MoveContainerOptions,
  MoveContainerPlan,
  MoveNetworkConfig,
  MoveProgressLogEntry,
  MoveVolumeConfig,
} from './move-container-types';

export type * from './move-container-types';

export function useMoveContainerPlan(
  id: string,
  targetEnvId: string,
  targetName: string,
  networkMode: string,
  networks: MoveNetworkConfig[],
  volumes: MoveVolumeConfig[],
  enabled: boolean,
) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-move-plan', envId, id, targetEnvId, targetName, networkMode, networks, volumes],
    queryFn: () => {
      const envQuery = envId ? `?env=${envId}` : '';
      return api
        .post<MoveContainerPlan>(`/containers/${id}/move/plan${envQuery}`, {
          targetEnvId,
          targetName,
          networkMode,
          networks,
          volumes,
        })
        .then(assertSuccess);
    },
    placeholderData: (previous) => previous,
    enabled: enabled && !!id && !!targetEnvId,
  });
}

function createMoveLogEntry(message: string, phase?: string): MoveProgressLogEntry {
  return {
    id: createClientId(),
    message,
    phase,
  };
}

function appendMoveLogEntry(entries: MoveProgressLogEntry[], message: string, phase?: string): MoveProgressLogEntry[] {
  const last = entries[entries.length - 1];
  if (last?.message === message && last.phase === phase) {
    return entries;
  }
  return [...entries, createMoveLogEntry(message, phase)];
}

function currentLanguage(): string {
  const stored = typeof window !== 'undefined' ? localStorage.getItem('mcharbor-language') : null;
  if (!stored) return 'en';
  try {
    return JSON.parse(stored)?.state?.language || 'en';
  } catch {
    return stored;
  }
}

export function useMoveContainerStream() {
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');
  const [moving, setMoving] = useState(false);
  const [progress, setProgress] = useState<MoveContainerEvent | null>(null);
  const [logs, setLogs] = useState<MoveProgressLogEntry[]>([]);
  const abortRef = useRef<AbortController | null>(null);

  const invalidateMoveQueries = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['containers'] });
    void queryClient.invalidateQueries({ queryKey: ['container'] });
    void queryClient.invalidateQueries({ queryKey: ['images'] });
    void queryClient.invalidateQueries({ queryKey: ['volumes'] });
    void queryClient.invalidateQueries({ queryKey: ['networks'] });
  }, [queryClient]);

  const reset = useCallback(() => {
    setMoving(false);
    setProgress(null);
    setLogs([]);
  }, []);

  const abort = useCallback(() => {
    abortRef.current?.abort();
    setMoving(false);
    setProgress({ step: 0, total: 10, message: t('moveDialog.progress.cancelled'), status: 'error', phase: 'error' });
    setLogs((prev) => appendMoveLogEntry(prev, t('moveDialog.progress.cancelled'), 'error'));
  }, [t]);

  const startMove = useCallback(
    (options: MoveContainerOptions, callbacks?: { onDone?: () => void }) => {
      const { id, ...body } = options;
      const envQuery = envId ? `?env=${envId}` : '';
      const controller = new AbortController();
      abortRef.current = controller;

      setMoving(true);
      setProgress({
        step: 0,
        total: 10,
        message: t('moveDialog.progress.starting'),
        status: 'progress',
        phase: 'start',
      });
      setLogs([createMoveLogEntry(t('moveDialog.progress.starting'), 'start')]);

      fetch(`/api/containers/${id}/move/stream${envQuery}`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          'Accept-Language': currentLanguage(),
        },
        body: JSON.stringify(body),
        signal: controller.signal,
      })
        .then((res) => {
          if (!res.ok || !res.body) {
            setMoving(false);
            setProgress({
              step: 0,
              total: 10,
              message: t('moveDialog.progress.connectFailed'),
              status: 'error',
              phase: 'error',
            });
            setLogs((prev) => appendMoveLogEntry(prev, t('moveDialog.progress.connectFailed'), 'error'));
            return;
          }

          const reader = res.body.getReader();
          const decoder = new TextDecoder();
          let buffer = '';
          let completed = false;

          const read = (): Promise<void> =>
            reader.read().then(({ done, value }) => {
              if (done) {
                setMoving(false);
                if (!completed) {
                  setProgress({
                    step: 0,
                    total: 10,
                    message: t('moveDialog.progress.networkError'),
                    status: 'error',
                    phase: 'error',
                  });
                  setLogs((prev) => appendMoveLogEntry(prev, t('moveDialog.progress.networkError'), 'error'));
                }
                return;
              }

              buffer += decoder.decode(value, { stream: true });
              const lines = buffer.split('\n');
              buffer = lines.pop() ?? '';

              for (const line of lines) {
                if (!line.startsWith('data: ')) continue;
                try {
                  const moveEvent = JSON.parse(line.slice(6)) as MoveContainerEvent;
                  setProgress(moveEvent);
                  if (moveEvent.message) {
                    setLogs((prev) => appendMoveLogEntry(prev, moveEvent.message, moveEvent.phase));
                  }
                  if (moveEvent.status === 'done') {
                    completed = true;
                    setMoving(false);
                    invalidateMoveQueries();
                    callbacks?.onDone?.();
                    return;
                  }
                  if (moveEvent.status === 'error') {
                    completed = true;
                    setMoving(false);
                    return;
                  }
                } catch {
                  // Skip malformed SSE lines.
                }
              }

              return read();
            });

          return read();
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === 'AbortError') return;
          setMoving(false);
          setProgress({
            step: 0,
            total: 10,
            message: t('moveDialog.progress.networkError'),
            status: 'error',
            phase: 'error',
          });
          setLogs((prev) => appendMoveLogEntry(prev, t('moveDialog.progress.networkError'), 'error'));
        });
    },
    [envId, invalidateMoveQueries, t],
  );

  return { moving, progress, logs, startMove, abort, reset };
}
