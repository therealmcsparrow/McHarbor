// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { createClientId } from '@resources/utils/id';
import type { ExecutionActions, ExecutionState } from './execution-store-types';

export function handleSSEEvent(
  eventName: string,
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

  switch (eventName) {
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

export function createSSEParser(onEvent: (eventName: string, data: string) => void): SSEParser {
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

export async function readExecutionError(res: Response): Promise<string> {
  try {
    const payload = (await res.json()) as { error?: string; message?: string };
    const detail = payload.error ?? payload.message ?? res.statusText;
    return `Execution failed: ${detail}`;
  } catch {
    return `Execution failed: ${res.statusText}`;
  }
}
