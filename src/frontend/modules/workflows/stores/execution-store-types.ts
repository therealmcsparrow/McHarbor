// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { NodeExecutionStatus } from '../types';
import type { DebugEntry } from '../components/DebugTab';
import type { ErrorEntry } from '../components/ErrorTab';

export type CloseableStream = {
  close: () => void;
};

export type ExecutionState = {
  isExecuting: boolean;
  nodeStates: Record<string, NodeExecutionStatus>;
  traversedEdges: Set<string>;
  animatingEdges: Set<string>;
  debugMessages: DebugEntry[];
  errors: ErrorEntry[];
  eventSource: CloseableStream | null;
};

export type ExecutionActions = {
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
