// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { createClientId } from '@resources/utils/id';
import { pluginLoader } from '@core/plugins/loader';
import type { PluginInstallResult } from '../types/plugins';
import type { PluginStreamEvent, PluginStreamState } from './streamPluginTypes';

export type { PluginStreamEvent, PluginStreamLogEntry, PluginStreamState } from './streamPluginTypes';

interface StreamPluginOptions {
  onSuccess?: (result: PluginInstallResult) => void;
  onError?: (code: string) => void;
}

export function useStreamPluginAction(options: StreamPluginOptions = {}) {
  const { t } = useTranslation('plugins');
  const qc = useQueryClient();
  const [state, setState] = useState<PluginStreamState>({
    running: false,
    progress: null,
    logs: [],
    result: null,
    errorCode: null,
  });
  const abortRef = useRef<AbortController | null>(null);
  const optionsRef = useRef(options);
  useEffect(() => {
    optionsRef.current = options;
  });

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setState({
      running: false,
      progress: null,
      logs: [],
      result: null,
      errorCode: null,
    });
  }, []);

  // A finished install/update has to land in the running SPA, not just
  // in the react-query cache: the sidebar, plugin routes, dashboard
  // widgets, workflow nodes, themes and i18n bundles all read the
  // Zustand plugin loader store, which otherwise keeps its boot-time
  // snapshot until a hard reload. `force` because the loader's `ready`
  // status short-circuits an unforced refresh.
  //
  // Permissions are invalidated too: install applies the manifest's
  // preset grants server-side, so the freshly contributed menu entry
  // would stay permission-hidden against the client's cached set.
  const finishInstall = useCallback(
    async (result: PluginInstallResult | null) => {
      await qc.invalidateQueries({ queryKey: ['appstore-plugins'] });
      await qc.invalidateQueries({ queryKey: ['auth', 'me', 'permissions'] });
      await pluginLoader.refresh({ force: true });
      if (result) optionsRef.current.onSuccess?.(result);
    },
    [qc],
  );

  const applyEvent = useCallback(
    (evt: PluginStreamEvent) => {
      const logLine = evt.message ?? (evt.status === 'error' ? evt.error_text : undefined);
      // State first, side effects after. Firing them from inside the
      // updater double-invokes them under StrictMode.
      setState((prev) => ({
        ...prev,
        running: evt.status === 'progress',
        progress: evt,
        result: evt.status === 'done' ? (evt.result ?? null) : prev.result,
        errorCode: evt.status === 'error' ? (evt.error_code ?? 'err.unknown') : prev.errorCode,
        logs: logLine
          ? [
              ...prev.logs,
              {
                id: createClientId(),
                message: logLine,
                phase: evt.phase,
              },
            ]
          : prev.logs,
      }));

      if (evt.status === 'done') {
        void finishInstall(evt.result ?? null);
      } else if (evt.status === 'error') {
        optionsRef.current.onError?.(evt.error_code ?? 'err.unknown');
      }
    },
    [finishInstall],
  );

  const runStream = useCallback(
    async (url: string, body: unknown) => {
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;

      const startingMessage = t('install.starting', {});
      setState({
        running: true,
        progress: {
          phase: 'starting',
          step: 0,
          total: 100,
          message: startingMessage,
          status: 'progress',
        },
        logs: [
          {
            id: createClientId(),
            message: startingMessage,
            phase: 'starting',
          },
        ],
        result: null,
        errorCode: null,
      });

      let res: Response;
      try {
        res = await fetch(url, {
          method: 'POST',
          credentials: 'include',
          headers: body ? { 'Content-Type': 'application/json' } : undefined,
          body: body ? JSON.stringify(body) : undefined,
          signal: controller.signal,
        });
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') return;
        const message = t('install.network_error', {});
        setState((prev) => ({
          ...prev,
          running: false,
          errorCode: 'err.network',
          progress: {
            phase: 'error',
            step: 0,
            total: 100,
            message,
            status: 'error',
          },
          logs: [...prev.logs, { id: createClientId(), message, phase: 'error' }],
        }));
        return;
      }

      if (!res.ok || !res.body) {
        const message = t('install.connect_failed', {});
        setState((prev) => ({
          ...prev,
          running: false,
          errorCode: 'err.connect_failed',
          progress: {
            phase: 'error',
            step: 0,
            total: 100,
            message,
            status: 'error',
          },
          logs: [...prev.logs, { id: createClientId(), message, phase: 'error' }],
        }));
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';
        for (const line of lines) {
          if (!line.startsWith('data: ')) continue;
          try {
            const eventItem = JSON.parse(line.slice(6)) as PluginStreamEvent;
            applyEvent(eventItem);
          } catch {
            // Malformed SSE line: skip it and keep consuming the stream.
          }
        }
      }
    },

    [t, applyEvent],
  );

  const startInstall = useCallback(
    (req: { url: string; checksum?: string; signature?: string; allow_unsigned?: boolean }) =>
      runStream('/api/appstore/plugins/install/stream', req),
    [runStream],
  );

  const startUpdate = useCallback(
    (slug: string, opts?: { allowUnsigned?: boolean }) =>
      runStream(
        `/api/appstore/plugins/${encodeURIComponent(slug)}/update/stream`,
        opts?.allowUnsigned ? { allow_unsigned: true } : undefined,
      ),
    [runStream],
  );

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return {
    state,
    startInstall,
    startUpdate,
    abort,
    reset,
  };
}
