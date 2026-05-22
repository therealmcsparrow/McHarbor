// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { createClientId } from '@resources/utils/id';
import type { NodeExecutionStatus } from '../types';
import type { DebugEntry } from '../components/DebugTab';
import type { ErrorEntry } from '../components/ErrorTab';

type ExecutionState = {
  isExecuting: boolean;
  nodeStates: Record<string, NodeExecutionStatus>;
  traversedEdges: Set<string>;
  animatingEdges: Set<string>;
  debugMessages: DebugEntry[];
  errors: ErrorEntry[];
  eventSource: CloseableStream | null;
};

type ExecutionActions = {
  startExecution: (workflowId: string, triggerNodeId: string) => void;
  stopExecution: () => void;
  resetExecution: () => void;
  subscribeLive: (workflowId: string) => void;
  unsubscribeLive: () => void;
  setNodeState: (nodeId: string, status: NodeExecutionStatus) => void;
  addTraversedEdge: (edgeId: string) => void;
  addDebugMessage: (entry: DebugEntry) => void;
  addError: (entry: ErrorEntry) => void;
  clearDebug: () => void;
  clearErrors: () => void;
};

type CloseableStream = {
  close: () => void;
};

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
    }).then(async (res) => {
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
      const parser = createSSEParser((event, data) => {
        handleSSEEvent(event, data, get, set);
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
    }).catch((err) => {
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

  setNodeState: (nodeId, status) => {
    set((s) => ({ nodeStates: { ...s.nodeStates, [nodeId]: status } }));
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

function handleSSEEvent(
  event: string,
  dataStr: string,
  get: () => ExecutionState & ExecutionActions,
  set: (fn: Partial<ExecutionState> | ((s: ExecutionState) => Partial<ExecutionState>)) => void,
) {
  let data: Record<string, unknown>;
  try {
    data = JSON.parse(dataStr);
  } catch {
    return;
  }

  const store = get();

  switch (event) {
    case 'node.started':
      store.setNodeState(data.nodeId as string, 'running');
      break;

    case 'node.completed':
      store.setNodeState(data.nodeId as string, 'completed');
      // Add as trace entry to debug messages
      store.addDebugMessage({
        id: createClientId(),
        type: 'trace',
        timestamp: new Date().toISOString(),
        nodeId: data.nodeId as string,
        nodeLabel: data.label as string,
        action: data.action as string,
        durationMs: data.durationMs as number,
        outputPort: data.outputPort as string,
        input: data.input,
        config: data.config,
        output: data.output,
      });
      break;

    case 'node.failed':
      store.setNodeState(data.nodeId as string, 'failed');
      store.addError({
        id: createClientId(),
        timestamp: new Date().toISOString(),
        nodeId: data.nodeId as string,
        nodeLabel: data.label as string,
        message: data.error as string,
      });
      break;

    case 'edge.traversed':
      store.addTraversedEdge(data.edgeId as string);
      break;

    case 'debug':
      store.addDebugMessage({
        id: createClientId(),
        type: 'debug',
        timestamp: new Date().toISOString(),
        source: (data.source as 'debug-node' | 'sniffer') ?? 'debug-node',
        nodeId: data.nodeId as string,
        nodeLabel: data.label as string,
        level: data.level as string,
        message: data.message as string,
        data: data.data,
      });
      break;

    case 'run.completed':
      set({ isExecuting: false, eventSource: null });
      break;

    case 'run.cancelled':
      set({ isExecuting: false, eventSource: null });
      break;

    case 'run.started':
      // For background-triggered runs, set executing state and clear previous visual state
      set((s) => ({
        isExecuting: true,
        nodeStates: {},
        traversedEdges: new Set(),
        animatingEdges: new Set(),
        debugMessages: s.debugMessages,
        errors: s.errors,
      }));
      break;
  }
}

type SSEParser = {
  push: (chunk: string) => void;
  flush: () => void;
};

function createSSEParser(onEvent: (event: string, data: string) => void): SSEParser {
  let buffer = '';
  let currentEvent = '';
  let currentData: string[] = [];

  const dispatch = () => {
    if (currentData.length === 0) {
      currentEvent = '';
      return;
    }

    onEvent(currentEvent || 'message', currentData.join('\n'));
    currentEvent = '';
    currentData = [];
  };

  const processLine = (rawLine: string) => {
    const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;

    if (line === '') {
      dispatch();
      return;
    }

    if (line.startsWith(':')) {
      return;
    }

    if (line.startsWith('event:')) {
      currentEvent = line.slice(6).trimStart();
      return;
    }

    if (line.startsWith('data:')) {
      currentData.push(line.slice(5).trimStart());
    }
  };

  return {
    push: (chunk) => {
      if (!chunk) {
        return;
      }

      buffer += chunk;
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const line of lines) {
        processLine(line);
      }
    },
    flush: () => {
      if (buffer) {
        processLine(buffer);
        buffer = '';
      }

      dispatch();
    },
  };
}

async function readExecutionError(res: Response): Promise<string> {
  try {
    const payload = await res.json() as { error?: string; message?: string };
    const detail = payload.error ?? payload.message ?? res.statusText;
    return `Execution failed: ${detail}`;
  } catch {
    return `Execution failed: ${res.statusText}`;
  }
}
