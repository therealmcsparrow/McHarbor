// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import {
  RETRYABLE_END_REASONS,
  isLogStreamRecord,
  parseEndReason,
  reconnectBackoff,
  retainLiveRecords,
  type LogConnectionState,
  type LogStreamRecord,
} from './containerLogsHelpers';

export type { LogConnectionState, LogStreamRecord };

export function useContainerLogs(containerId: string, tail = 500, live = false, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const [liveRecords, setLiveRecords] = useState<LogStreamRecord[]>([]);
  const [connectionState, setConnectionState] = useState<LogConnectionState>('idle');
  const [reconnectDelay, setReconnectDelay] = useState(0);
  const [liveError, setLiveError] = useState<Error | null>(null);
  const [retryVersion, setRetryVersion] = useState(0);
  const sourceRef = useRef<EventSource | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stableTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cursorRef = useRef('');
  const recordIdsRef = useRef(new Set<string>());
  const pendingRecordsRef = useRef<LogStreamRecord[]>([]);
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const snapshot = useQuery({
    queryKey: ['container-logs', envId, containerId, tail],
    queryFn: () =>
      api
        .get<{ logs: string }>(`/containers/${containerId}/logs`, {
          tail: String(tail),
          timestamps: 'true',
          ...(envId ? { env: envId } : {}),
        })
        .then(assertSuccess)
        .then((data) => data.logs),
    enabled: enabled && !!containerId && !live,
  });

  const streamKey = `${containerId}|${enabled}|${envId}|${live}|${retryVersion}|${tail}`;
  const [appliedStreamKey, setAppliedStreamKey] = useState(streamKey);
  if (appliedStreamKey !== streamKey) {
    setAppliedStreamKey(streamKey);
    setLiveRecords([]);
    setConnectionState(enabled && live ? 'connecting' : 'idle');
    setReconnectDelay(0);
    setLiveError(null);
  }

  useEffect(() => {
    cursorRef.current = '';
    recordIdsRef.current.clear();
    pendingRecordsRef.current = [];

    if (!enabled || !live || !containerId) return;

    let disposed = false;
    let reconnectAttempt = 0;

    const cleanupSource = () => {
      if (stableTimerRef.current) {
        clearTimeout(stableTimerRef.current);
        stableTimerRef.current = null;
      }
      sourceRef.current?.close();
      sourceRef.current = null;
    };

    const flushRecords = () => {
      flushTimerRef.current = null;
      const pendingRecords = pendingRecordsRef.current;
      pendingRecordsRef.current = [];
      if (pendingRecords.length === 0) return;
      setLiveRecords((current) => {
        const next = retainLiveRecords([...current, ...pendingRecords]);
        recordIdsRef.current = new Set(next.map((record) => record.id));
        return next;
      });
    };

    const connect = () => {
      if (disposed) return;
      cleanupSource();
      const params = new URLSearchParams({
        tail: String(Math.min(Math.max(tail, 1), 2000)),
        follow: 'true',
        stdout: 'true',
        stderr: 'true',
      });
      if (envId) params.set('env', envId);
      if (cursorRef.current) params.set('cursor', cursorRef.current);

      setConnectionState(reconnectAttempt > 0 ? 'reconnecting' : 'connecting');
      const source = new EventSource(`/api/logs/${encodeURIComponent(containerId)}?${params.toString()}`);
      sourceRef.current = source;

      source.addEventListener('connected', () => {
        if (disposed || sourceRef.current !== source) return;
        setReconnectDelay(0);
        setLiveError(null);
        setConnectionState('connected');
        stableTimerRef.current = setTimeout(() => {
          reconnectAttempt = 0;
          stableTimerRef.current = null;
        }, 30_000);
      });

      source.addEventListener('log', (evt) => {
        if (disposed || sourceRef.current !== source) return;
        try {
          const value: unknown = JSON.parse(evt.data);
          if (!isLogStreamRecord(value)) {
            setLiveError(new Error());
            setConnectionState('error');
            return;
          }
          cursorRef.current = evt.lastEventId || value.id;
          if (recordIdsRef.current.has(value.id)) return;
          recordIdsRef.current.add(value.id);
          pendingRecordsRef.current.push(value);
          if (!flushTimerRef.current) {
            flushTimerRef.current = setTimeout(flushRecords, 100);
          }
        } catch {
          setLiveError(new Error());
          setConnectionState('error');
        }
      });

      source.addEventListener('end', (evt) => {
        if (disposed || sourceRef.current !== source) return;
        const reason = parseEndReason(evt.data);
        cleanupSource();
        if (RETRYABLE_END_REASONS.includes(reason)) {
          reconnectAttempt++;
          const delay = reconnectBackoff(reconnectAttempt);
          setReconnectDelay(delay);
          setConnectionState('reconnecting');
          timerRef.current = setTimeout(connect, delay);
        } else {
          setConnectionState('ended');
        }
      });

      source.addEventListener('stream-error', () => {
        if (!disposed && sourceRef.current === source) {
          setLiveError(new Error());
          setConnectionState('error');
        }
      });

      source.onerror = () => {
        if (disposed || sourceRef.current !== source) return;
        cleanupSource();
        reconnectAttempt++;
        const delay = reconnectBackoff(reconnectAttempt);
        setReconnectDelay(delay);
        setConnectionState('reconnecting');
        timerRef.current = setTimeout(connect, delay);
      };
    };

    connect();
    return () => {
      disposed = true;
      if (timerRef.current) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      if (flushTimerRef.current) {
        clearTimeout(flushTimerRef.current);
        flushTimerRef.current = null;
      }
      pendingRecordsRef.current = [];
      cleanupSource();
    };
  }, [containerId, enabled, envId, live, retryVersion, tail]);

  const liveData = liveRecords.map((record) => `${record.timestamp} ${record.message}`).join('\n');

  return {
    ...snapshot,
    data: live ? liveData : (snapshot.data ?? ''),
    isLoading: live ? connectionState === 'connecting' && liveRecords.length === 0 : snapshot.isLoading,
    error: live ? liveError : snapshot.error,
    isError: live ? liveError !== null : snapshot.isError,
    refetch: live
      ? async () => {
          setRetryVersion((current) => current + 1);
        }
      : snapshot.refetch,
    records: liveRecords,
    connectionState,
    reconnectDelay,
  };
}
