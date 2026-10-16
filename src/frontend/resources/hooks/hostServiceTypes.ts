// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type HostServiceAvailabilityReason =
  | 'not_agent'
  | 'not_native'
  | 'offline'
  | 'agent_outdated'
  | 'no_systemd'
  | 'self_unreachable'
  | 'self_no_systemd';

export type HostServiceAvailability = {
  supported: boolean;
  reason?: HostServiceAvailabilityReason;
  detail?: string;
  installKind?: string;
  agentVersion?: string;
  agentBuild?: string;
  minVersion: string;
  minBuild: string;
};

export type HostServiceProtection = 'agent' | 'ssh' | 'systemd' | 'dbus' | 'runtime' | 'mcharbor' | 'self_runtime';

export type HostService = {
  name: string;
  description?: string;
  loadState?: string;
  activeState?: string;
  subState?: string;
  unitFileState?: string;
  mainPid?: number;
  result?: string;
  failureReason?: string;
  template?: boolean;
  protected?: HostServiceProtection;
  isAgent?: boolean;
  restartsMcHarbor?: boolean;
};

export type HostServiceList = {
  availability: HostServiceAvailability;
  units: HostService[];
  truncated?: boolean;
};

export type HostServiceAction = 'start' | 'stop' | 'restart' | 'enable' | 'disable';

export type HostServiceActionResult = {
  unit: string;
  action: HostServiceAction;
  warning?: 'agent_restart' | 'mcharbor_restart';
};

export type HostServiceOverride = {
  unit: string;
  effective: string;
  content: string;
  exists: boolean;
  path: string;
  truncated?: boolean;
};

export type HostServiceOverrideInput = {
  unit: string;
  content: string;
  reload: boolean;
  verify: boolean;
};

export type HostServiceOverrideResult = {
  unit: string;
  path: string;
  reloaded: boolean;
  verifyOutput?: string;
  sha256: string;
};
