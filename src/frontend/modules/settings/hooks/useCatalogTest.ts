// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';
import type { CatalogKind } from './useCatalogs';

// Step keys, in the order the backend walks them
// (modules/catalogs/test.go → TestSteps). The dialog renders one row per
// key before the run starts, so the plan is visible up front and each
// row fills in as its event arrives.
export const CATALOG_TEST_STEPS = ['config', 'transport', 'reach', 'read', 'content'] as const;
export type CatalogTestStepKey = (typeof CATALOG_TEST_STEPS)[number];

export type CatalogTestStatus = 'pending' | 'running' | 'ok' | 'warn' | 'fail' | 'skipped';

export interface CatalogTestProblem {
  file: string;
  reason_key: string;
}

// Mirrors the Go TestStep struct. detail_key is an i18n key suffix under
// `catalogs.test.detail.*` and detail_args its interpolation values — the
// backend never sends prose, so every line renders in the user's locale.
export interface CatalogTestStep {
  key: CatalogTestStepKey;
  status: Exclude<CatalogTestStatus, 'pending'>;
  detail_key?: string;
  detail_args?: Record<string, string | number>;
  samples?: string[];
  problems?: CatalogTestProblem[];
  duration_ms: number;
}

export interface CatalogTestResult {
  catalog_id: string;
  kind: CatalogKind;
  url: string;
  status: 'ok' | 'warn' | 'fail';
  entries: number;
  steps: CatalogTestStep[];
}

interface CatalogTestEvent {
  status: 'progress' | 'done' | 'error';
  index: number;
  total: number;
  step?: CatalogTestStep;
  result?: CatalogTestResult;
  error_code?: string;
}

export interface CatalogTestState {
  running: boolean;
  // steps is keyed by step key rather than an array so a late event for
  // an earlier step cannot reorder the rows.
  steps: Partial<Record<CatalogTestStepKey, CatalogTestStep>>;
  result: CatalogTestResult | null;
  errorCode: string | null;
}

const IDLE: CatalogTestState = {
  running: false,
  steps: {},
  result: null,
  errorCode: null,
};

/**
 * Runs the catalog probe against `POST /api/catalogs/{id}/test/stream`
 * and accumulates the SSE step events.
 *
 * The endpoint streams rather than returning one JSON body because the
 * remote branch can spend up to 15s on the fetch — the whole point of
 * the dialog is showing which step is spending it. Reading the stream
 * by hand (`fetch` + `ReadableStream`) rather than with `EventSource`
 * follows the existing install/uninstall streams: `EventSource` cannot
 * issue a POST.
 */
export function useCatalogTest() {
  const [state, setState] = useState<CatalogTestState>(IDLE);
  const abortRef = useRef<AbortController | null>(null);

  // Abort any in-flight probe when the dialog unmounts, so a closed
  // modal does not keep a connection (and a setState) alive.
  useEffect(() => {
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
    };
  }, []);

  const reset = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setState(IDLE);
  }, []);

  const applyEvent = useCallback((streamEvent: CatalogTestEvent) => {
    setState((prev) => {
      const next: CatalogTestState = { ...prev, steps: { ...prev.steps } };
      if (streamEvent.step) {
        next.steps[streamEvent.step.key] = streamEvent.step;
      }
      if (streamEvent.status === 'done') {
        next.running = false;
        next.result = streamEvent.result ?? null;
      } else if (streamEvent.status === 'error') {
        next.running = false;
        next.errorCode = streamEvent.error_code ?? 'err.unknown';
      }
      return next;
    });
  }, []);

  const start = useCallback(
    async (catalogId: string) => {
      abortRef.current?.abort();
      const controller = new AbortController();
      abortRef.current = controller;
      setState({ ...IDLE, running: true, steps: {} });

      let res: Response;
      try {
        res = await fetch(`/api/catalogs/${encodeURIComponent(catalogId)}/test/stream`, {
          method: 'POST',
          credentials: 'include',
          signal: controller.signal,
        });
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setState((prev) => ({ ...prev, running: false, errorCode: 'err.network' }));
        return;
      }

      if (!res.ok || !res.body) {
        setState((prev) => ({
          ...prev,
          running: false,
          errorCode: res.status === 403 ? 'err.forbidden' : 'err.connect_failed',
        }));
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      try {
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() ?? '';
          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;
            try {
              applyEvent(JSON.parse(line.slice(6)) as CatalogTestEvent);
            } catch {
              // Ignore a malformed frame; the stream may continue with
              // valid events and one bad line should not end the run.
            }
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setState((prev) => ({ ...prev, running: false, errorCode: 'err.network' }));
        return;
      }

      // The stream closed without a terminal event — the server went
      // away mid-run. Stop the spinners rather than leaving them turning.
      setState((prev) => (prev.running ? { ...prev, running: false, errorCode: 'err.stream_closed' } : prev));
    },
    [applyEvent],
  );

  return { state, start, reset };
}
