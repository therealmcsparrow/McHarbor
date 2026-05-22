// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ConfigFieldType = 'text' | 'textarea' | 'number' | 'select' | 'toggle' | 'json' | 'key-value' | 'expression' | 'cron' | 'container-select' | 'environment-select' | 'metric-conditions' | 'code' | 'link-output-select' | 'email-server-select' | 'communication-channel-select';

export type ConfigFieldOption = {
  value: string;
  label: string;
};

export type ConfigField = {
  key: string;
  label: string;
  type: ConfigFieldType;
  required: boolean;
  secret?: boolean;
  options?: ConfigFieldOption[];
  default?: unknown;
  showWhen?: Record<string, string>;
};

export type NodeDefinition = {
  key: string;
  label: string;
  category: string;
  description: string;
  icon: string;
  configSchema: ConfigField[];
  inputPorts: string[];
  outputPorts: string[];
  requires?: string | string[];
};

export type CanvasNode = {
  id: string;
  type: string;
  action: string;
  label: string;
  config: Record<string, unknown>;
  position: { x: number; y: number };
  portLabels?: Record<string, string>;
  blockedPorts?: string[];
  debug?: boolean;
  skip?: boolean;
  disabled?: boolean;
};

export type CanvasEdge = {
  id: string;
  sourceNodeId: string;
  sourcePort: string;
  targetNodeId: string;
  targetPort: string;
  label?: string;
  labelOffset?: { x: number; y: number };
  sniffer?: { name: string };
  snifferOffset?: { x: number; y: number };
};

export type CanvasViewport = {
  x: number;
  y: number;
  zoom: number;
};

export type CanvasGroup = {
  id: string;
  name: string;
  color: string;
  nodeIds: string[];
  blocked?: boolean;
};

export type CanvasData = {
  nodes: CanvasNode[];
  edges: CanvasEdge[];
  groups: CanvasGroup[];
  viewport: CanvasViewport;
};

export type Workflow = {
  id: string;
  name: string;
  description: string;
  status: string;
  canvasData: string;
  variables: string;
  createdBy: string;
  updatedBy: string;
  lastRunAt: string;
  createdAt: string;
  updatedAt: string;
};

export type ExtraCondition = {
  field: string;
  operator: string;
  value: string;
  label: string;
};

export type SwitchCase = {
  value: string;
  label: string;
};

export type NodeExecutionStatus = 'idle' | 'running' | 'completed' | 'failed';

export function getEffectiveInputPorts(node: CanvasNode, definition?: NodeDefinition): string[] {
  if (node.action === 'join') {
    const count = Math.max(2, Math.min(Number(node.config.input_count) || 2, 16));
    return Array.from({ length: count }, (_, i) => `input_${i}`);
  }
  return definition?.inputPorts ?? ['input'];
}

export function getEffectiveOutputPorts(node: CanvasNode, definition?: NodeDefinition): string[] {
  if (node.action === 'condition') {
    const extras = (node.config.extra_conditions ?? []) as ExtraCondition[];
    if (extras.length > 0) {
      const ports = ['true'];
      extras.forEach((_, i) => ports.push(`condition_${i}`));
      ports.push('else');
      return ports;
    }
  }
  if (node.action === 'switch') {
    const cases = (node.config.switch_cases ?? []) as SwitchCase[];
    const ports = cases.map((_, i) => `case_${i}`);
    if (ports.length === 0) ports.push('case_0');
    ports.push('default');
    return ports;
  }
  return definition?.outputPorts ?? ['output'];
}

/** Node-RED style message object passed between workflow nodes. */
export type WorkflowMsg = {
  _msgid: string;
  topic?: string;
  payload: unknown;
  req?: { method?: string; url?: string; headers?: Record<string, string>; params?: Record<string, string>; query?: Record<string, string>; body?: unknown };
  res?: { status?: number };
  statusCode?: number;
  headers?: Record<string, string>;
  parts?: { id?: string; type?: string; count?: number; index?: number; key?: string };
  socket?: { remoteAddress?: string; remotePort?: number };
  _performance?: Record<string, { durationMs: number }>;
  [key: string]: unknown;
};

export function getNodeHeightForPorts(portCount: number): number {
  return Math.max(76, (portCount + 1) * 22);
}

export const JUNCTION_SIZE = 20;
