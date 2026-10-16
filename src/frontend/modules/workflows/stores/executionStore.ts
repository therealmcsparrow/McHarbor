// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { createClientId } from '@resources/utils/id';
import type { ExecutionActions, ExecutionState } from './execution-store-types';
import { createSSEParser, handleSSEEvent, readExecutionError } from './execution-sse';

let animationTimers: Record<string, ReturnType<typeof setTimeout>> = {};
let liveEventSource: EventSource | null = null;

export const useExecutionStore = create<ExecutionState & ExecutionActions>((set, get) => ({
  isExecuting: false,
  nodeStates: {},
  traversedEdges: new Set(),
  animatingEdges: new Set(),
  debugMessages: [],
  errors: [],
  eventSource: null,

  startExecution: (workflowId, triggerNodeId) => {
    // Close any existing connection
    get().eventSource?.close();

    // Reset visual state but preserve debug/error history
    set((s) => ({
      isExecuting: true,
      nodeStates: {},
      traversedEdges: new Set(),
      animatingEdges: new Set(),
      debugMessages: s.debugMessages,
      errors: s.errors,
    }));

    // Clear animation timers
    for (const key of Object.keys(animationTimers)) {
      clearTimeout(animationTimers[key]);
    }
    animationTimers = {};

    // POST to execute endpoint and parse its SSE stream for manual runs.
    const url = `/api/workflows/${workflowId}/execute`;

    const abortController = new AbortController();

    fetch(url, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ triggerNodeId }),
      signal: abortController.signal,
    })
      .then(async (res) => {
        if (!res.ok || !res.body) {
          const message = await readExecutionError(res);
          set({ isExecuting: false, eventSource: null });
          get().addError({
            id: createClientId(),
            timestamp: new Date().toISOString(),
            message,
          });
          return;
        }

        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        const parser = createSSEParser((eventName, payload) => {
          handleSSEEvent(eventName, payload, get, set);
        });

        while (true) {
          const { done, value } = await reader.read();
          if (done) {
            parser.flush();
            break;
          }

          parser.push(decoder.decode(value, { stream: true }));
        }

        set({ eventSource: null });
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          set({ isExecuting: false });
          get().addError({
            id: createClientId(),
            timestamp: new Date().toISOString(),
            message: `Connection error: ${err.message}`,
          });
        }
      });

    // Store abort controller as the "event source" cleanup mechanism
    set({ eventSource: { close: () => abortController.abort() } });
  },

  stopExecution: () => {
    get().eventSource?.close();
    set({ isExecuting: false, eventSource: null });
    for (const key of Object.keys(animationTimers)) {
      clearTimeout(animationTimers[key]);
    }
    animationTimers = {};
  },

  resetExecution: () => {
    get().eventSource?.close();
    liveEventSource?.close();
    liveEventSource = null;
    for (const key of Object.keys(animationTimers)) {
      clearTimeout(animationTimers[key]);
    }
    animationTimers = {};
    set({
      isExecuting: false,
      nodeStates: {},
      traversedEdges: new Set(),
      animatingEdges: new Set(),
      debugMessages: [],
      errors: [],
      eventSource: null,
    });
  },

  subscribeLive: (workflowId) => {
    // Close any existing live connection
    liveEventSource?.close();

    const es = new EventSource(`/api/workflows/${workflowId}/live`);
    liveEventSource = es;

    const handler = (e: MessageEvent) => {
      handleSSEEvent(e.type, e.data, get, set);
    };

    es.addEventListener('run.started', handler);
    es.addEventListener('run.completed', handler);
    es.addEventListener('run.cancelled', handler);
    es.addEventListener('node.started', handler);
    es.addEventListener('node.completed', handler);
    es.addEventListener('node.failed', handler);
    es.addEventListener('edge.traversed', handler);
    es.addEventListener('debug', handler);

    es.onerror = () => {
      es.close();
      // Reconnect after 3 seconds
      setTimeout(() => {
        if (liveEventSource === es) {
          get().subscribeLive(workflowId);
        }
      }, 3000);
    };
  },

  unsubscribeLive: () => {
    liveEventSource?.close();
    liveEventSource = null;
  },

  setNodeState: (nodeId, nodeStatus) => {
    set((s) => ({ nodeStates: { ...s.nodeStates, [nodeId]: nodeStatus } }));
  },

  addTraversedEdge: (edgeId) => {
    set((s) => {
      const traversed = new Set(s.traversedEdges);
      const animating = new Set(s.animatingEdges);
      traversed.add(edgeId);
      animating.add(edgeId);
      return { traversedEdges: traversed, animatingEdges: animating };
    });

    // Stop animation after 3 seconds
    animationTimers[edgeId] = setTimeout(() => {
      set((s) => {
        const animating = new Set(s.animatingEdges);
        animating.delete(edgeId);
        return { animatingEdges: animating };
      });
      delete animationTimers[edgeId];
    }, 3000);
  },

  addDebugMessage: (entry) => {
    set((s) => ({ debugMessages: [...s.debugMessages, entry] }));
  },

  addError: (entry) => {
    set((s) => ({ errors: [...s.errors, entry] }));
  },

  clearDebug: () => set({ debugMessages: [] }),

  clearErrors: () => set({ errors: [] }),
}));
