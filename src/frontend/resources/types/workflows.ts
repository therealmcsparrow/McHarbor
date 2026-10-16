// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';

export type ConfigFieldType =
  | 'text'
  | 'textarea'
  | 'number'
  | 'select'
  | 'toggle'
  | 'json'
  | 'key-value'
  | 'expression'
  | 'cron'
  | 'container-select'
  | 'environment-select'
  | 'metric-conditions'
  | 'code'
  | 'link-output-select'
  | 'email-server-select'
  | 'communication-channel-select'
  | 'webhook-select'
  | 'registry-select'
  | `storage-location-select`
  | 'stack-select'
  | 'image-select'
  | 'volume-select'
  | 'network-select'
  | 'backup-plan-select'
  | 'backup-run-select'
  | 'multiselect'
  | 'timezone-select'
  | 'container-mount-multiselect'
  | 'backup-restore-item-multiselect'
  | 'stack-service-select'
  | 'plugin-select'
  | 'image-scanner-select';

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
  channelType?: string;
  incidentOnly?: boolean;
  /**
   * Resource selectors, `select` and `multiselect`: let the operator type
   * a value that is not in the list (a stack that does not exist yet, an
   * image to pull, a MIME type outside the presets). On `select` this
   * swaps the dropdown for a resource picker with a custom-value entry;
   * on `multiselect` typed entries are added as extra selections.
   */
  allowCustom?: boolean;
  /**
   * Conditional visibility. Every key must match; a string array means
   * "any of these values".
   */
  showWhen?: Record<string, string | string[]>;
  /**
   * `plugin-select` only: the picker-source the options come from. The host
   * loader pins `plugin` to the plugin that registered the node.
   */
  source?: { plugin: string; target: string };
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
  /**
   * RBAC permission key a plugin-contributed node is gated on
   * (`requires_permission` from the plugin loader). Distinct from
   * `requires`, which names service capabilities: a user lacking
   * this key sees the node disabled in the palette with a tooltip
   * naming the permission, exactly like a gated button.
   */
  permission?: string;
  /**
   * Optional React component used by the workflow canvas to render
   * the node body. Built-in nodes omit this (the canvas renders its
   * default body); plugin-supplied nodes attach the component the
   * plugin's UMD bundle exports under `nodes.<key>`.
   */
  component?: ComponentType<unknown>;
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
  /** Set by the server when the last save was by a host_self.manage holder; runs may then act on McHarbor itself. */
  selfHostAuthorized?: boolean;
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
  req?: {
    method?: string;
    url?: string;
    headers?: Record<string, string>;
    params?: Record<string, string>;
    query?: Record<string, string>;
    body?: unknown;
  };
  res?: { status?: number };
  statusCode?: number;
  headers?: Record<string, string>;
  parts?: {
    id?: string;
    type?: string;
    count?: number;
    index?: number;
    key?: string;
  };
  socket?: { remoteAddress?: string; remotePort?: number };
  _performance?: Record<string, { durationMs: number }>;
  [key: string]: unknown;
};

export function getNodeHeightForPorts(portCount: number): number {
  return Math.max(76, (portCount + 1) * 22);
}

export const JUNCTION_SIZE = 20;
