// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect } from '@core/types/docker';

export type PortMappingEntry = {
  containerPort: string;
  hostPort: string;
  hostIp: string;
  protocol: string;
};

export type HealthcheckConfig = {
  enabled: boolean;
  command: string;
  interval: number;
  timeout: number;
  retries: number;
  startPeriod: number;
};

type DeviceRequestPayload = {
  Driver: string;
  Count: number;
  DeviceIDs?: string[];
  Capabilities: string[][];
};

export type EditFormData = {
  // General / Image
  image: string;

  // Environment tab
  env: string[];
  labels: Record<string, string>;
  cmd: string[];
  entrypoint: string[];
  workingDir: string;
  hostname: string;
  domainname: string;
  user: string;
  tty: boolean;
  openStdin: boolean;
  stopSignal: string;

  // Healthcheck
  healthcheck: HealthcheckConfig;

  // Network tab
  portMappings: PortMappingEntry[];
  exposedPorts: string[];
  dns: string[];
  dnsSearch: string[];
  dnsOptions: string[];
  extraHosts: string[];
  networkMode: string;

  // Resources tab
  restartPolicyName: string;
  restartPolicyMaxRetry: number;
  memory: number;
  memorySwap: number;
  memoryReservation: number;
  nanoCpus: number;
  cpuShares: number;
  cpuPeriod: number;
  cpuQuota: number;
  cpusetCpus: string;
  cpusetMems: string;
  blkioWeight: number;
  privileged: boolean;
  readonlyRootfs: boolean;
  capAdd: string[];
  capDrop: string[];
  securityOpt: string[];
  shmSize: number;
  pidMode: string;
  init: boolean;
  autoRemove: boolean;
  oomKillDisable: boolean;
  pidsLimit: number;
  gpuEnabled: boolean;
  gpuDriver: string;
  gpuCount: number;
  gpuDeviceIds: string[];
  gpuCapabilities: string[];

  // Logging
  logDriver: string;
  logOptions: Record<string, string>;
};

export type ChangeClassification = {
  hasResourceChanges: boolean;
  hasConfigChanges: boolean;
  changedResourceFields: string[];
  changedConfigFields: string[];
};

const RESOURCE_FIELDS = new Set([
  'memory', 'memorySwap', 'memoryReservation', 'nanoCpus',
  'cpuShares', 'cpuPeriod', 'cpuQuota', 'cpusetCpus', 'cpusetMems',
  'blkioWeight', 'restartPolicyName', 'restartPolicyMaxRetry',
]);

function parsePortMappings(ports: ContainerInspect['NetworkSettings']['Ports']): PortMappingEntry[] {
  if (!ports) return [];
  const entries: PortMappingEntry[] = [];
  for (const [containerPortKey, bindings] of Object.entries(ports)) {
    if (!bindings || bindings.length === 0) continue;
    const parts = containerPortKey.split('/');
    const portNum = parts[0] ?? '';
    const proto = parts[1] ?? 'tcp';
    for (const b of bindings) {
      entries.push({
        containerPort: portNum,
        hostPort: b.HostPort,
        hostIp: b.HostIp || '0.0.0.0',
        protocol: proto,
      });
    }
  }
  return entries;
}

export function containerToEditForm(c: ContainerInspect): EditFormData {
  const hc = c.HostConfig;
  const hcheck = c.Config?.Healthcheck;
  const gpuRequest = hc?.DeviceRequests?.[0];
  const gpuCapabilities = gpuRequest?.Capabilities?.flat().filter(Boolean) ?? ['gpu'];
  const gpuDeviceIds = gpuRequest?.DeviceIDs?.filter(Boolean) ?? [];
  const gpuCount =
    typeof gpuRequest?.Count === 'number' && gpuRequest.Count !== 0 ? gpuRequest.Count : -1;

  return {
    image: c.Config?.Image ?? '',

    env: c.Config?.Env ?? [],
    labels: { ...(c.Config?.Labels ?? {}) },
    cmd: c.Config?.Cmd ?? [],
    entrypoint: c.Config?.Entrypoint ?? [],
    workingDir: c.Config?.WorkingDir ?? '',
    hostname: c.Config?.Hostname ?? '',
    domainname: c.Config?.Domainname ?? '',
    user: c.Config?.User ?? '',
    tty: c.Config?.Tty ?? false,
    openStdin: c.Config?.OpenStdin ?? false,
    stopSignal: c.Config?.StopSignal ?? '',

    healthcheck: {
      enabled: !!(hcheck?.Test && hcheck.Test.length > 0 && hcheck.Test[0] !== 'NONE'),
      command: hcheck?.Test?.slice(1).join(' ') ?? '',
      interval: hcheck?.Interval ? hcheck.Interval / 1e9 : 30,
      timeout: hcheck?.Timeout ? hcheck.Timeout / 1e9 : 30,
      retries: hcheck?.Retries ?? 3,
      startPeriod: hcheck?.StartPeriod ? hcheck.StartPeriod / 1e9 : 0,
    },

    portMappings: parsePortMappings(c.NetworkSettings?.Ports),
    exposedPorts: c.Config?.ExposedPorts ? Object.keys(c.Config.ExposedPorts) : [],
    dns: hc?.Dns ?? [],
    dnsSearch: hc?.DnsSearch ?? [],
    dnsOptions: hc?.DnsOptions ?? [],
    extraHosts: hc?.ExtraHosts ?? [],
    networkMode: hc?.NetworkMode ?? '',

    restartPolicyName: hc?.RestartPolicy?.Name ?? 'no',
    restartPolicyMaxRetry: hc?.RestartPolicy?.MaximumRetryCount ?? 0,
    memory: hc?.Memory ?? 0,
    memorySwap: hc?.MemorySwap ?? 0,
    memoryReservation: hc?.MemoryReservation ?? 0,
    nanoCpus: hc?.NanoCpus ?? 0,
    cpuShares: hc?.CpuShares ?? 0,
    cpuPeriod: hc?.CpuPeriod ?? 0,
    cpuQuota: hc?.CpuQuota ?? 0,
    cpusetCpus: hc?.CpusetCpus ?? '',
    cpusetMems: hc?.CpusetMems ?? '',
    blkioWeight: hc?.BlkioWeight ?? 0,
    privileged: hc?.Privileged ?? false,
    readonlyRootfs: hc?.ReadonlyRootfs ?? false,
    capAdd: hc?.CapAdd ?? [],
    capDrop: hc?.CapDrop ?? [],
    securityOpt: hc?.SecurityOpt ?? [],
    shmSize: hc?.ShmSize ?? 0,
    pidMode: hc?.PidMode ?? '',
    init: hc?.Init ?? false,
    autoRemove: hc?.AutoRemove ?? false,
    oomKillDisable: hc?.OomKillDisable ?? false,
    pidsLimit: typeof hc?.PidsLimit === 'number' ? hc.PidsLimit : 0,
    gpuEnabled: !!gpuRequest,
    gpuDriver: gpuRequest?.Driver ?? 'nvidia',
    gpuCount,
    gpuDeviceIds,
    gpuCapabilities,

    logDriver: hc?.LogConfig?.Type ?? '',
    logOptions: { ...(hc?.LogConfig?.Config ?? {}) },
  };
}

export function classifyChanges(
  original: EditFormData,
  current: EditFormData,
): ChangeClassification {
  const changedResourceFields: string[] = [];
  const changedConfigFields: string[] = [];

  for (const key of Object.keys(original) as Array<keyof EditFormData>) {
    const orig = original[key];
    const curr = current[key];
    if (JSON.stringify(orig) === JSON.stringify(curr)) continue;

    if (RESOURCE_FIELDS.has(key)) {
      changedResourceFields.push(key);
    } else {
      changedConfigFields.push(key);
    }
  }

  return {
    hasResourceChanges: changedResourceFields.length > 0,
    hasConfigChanges: changedConfigFields.length > 0,
    changedResourceFields,
    changedConfigFields,
  };
}

export function buildUpdatePayload(
  data: EditFormData,
  changes: ChangeClassification,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};

  for (const field of changes.changedResourceFields) {
    switch (field) {
      case 'memory': payload.memory = data.memory; break;
      case 'memorySwap': payload.memorySwap = data.memorySwap; break;
      case 'memoryReservation': payload.memoryReservation = data.memoryReservation; break;
      case 'nanoCpus': payload.nanoCPUs = data.nanoCpus; break;
      case 'cpuShares': payload.cpuShares = data.cpuShares; break;
      case 'cpuPeriod': payload.cpuPeriod = data.cpuPeriod; break;
      case 'cpuQuota': payload.cpuQuota = data.cpuQuota; break;
      case 'cpusetCpus': payload.cpusetCpus = data.cpusetCpus; break;
      case 'cpusetMems': payload.cpusetMems = data.cpusetMems; break;
      case 'blkioWeight': payload.blkioWeight = data.blkioWeight; break;
      case 'restartPolicyName':
      case 'restartPolicyMaxRetry':
        payload.restartPolicy = {
          Name: data.restartPolicyName,
          MaximumRetryCount: data.restartPolicyMaxRetry,
        };
        break;
    }
  }

  return payload;
}

function portMappingsToBindings(mappings: PortMappingEntry[]): Record<string, Array<{ HostIp: string; HostPort: string }>> {
  const result: Record<string, Array<{ HostIp: string; HostPort: string }>> = {};
  for (const m of mappings) {
    const key = `${m.containerPort}/${m.protocol}`;
    if (!result[key]) result[key] = [];
    result[key].push({ HostIp: m.hostIp, HostPort: m.hostPort });
  }
  return result;
}

function buildDeviceRequestsPayload(data: EditFormData): DeviceRequestPayload[] {
  if (!data.gpuEnabled) {
    return [];
  }

  const capabilities = data.gpuCapabilities.filter(Boolean);
  const deviceIds = data.gpuDeviceIds.filter(Boolean);

  return [
    {
      Driver: data.gpuDriver.trim() || 'nvidia',
      Count: deviceIds.length > 0 ? 0 : data.gpuCount,
      ...(deviceIds.length > 0 ? { DeviceIDs: deviceIds } : {}),
      Capabilities: [capabilities.length > 0 ? capabilities : ['gpu']],
    },
  ];
}

export function buildRecreatePayload(
  data: EditFormData,
  changes: ChangeClassification,
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  const fields = new Set(changes.changedConfigFields);

  if (fields.has('image')) payload.image = data.image;
  if (fields.has('env')) payload.env = data.env;
  if (fields.has('labels')) payload.labels = data.labels;
  if (fields.has('cmd')) payload.cmd = data.cmd;
  if (fields.has('entrypoint')) payload.entrypoint = data.entrypoint;
  if (fields.has('workingDir')) payload.workingDir = data.workingDir;
  if (fields.has('hostname')) payload.hostname = data.hostname;
  if (fields.has('domainname')) payload.domainname = data.domainname;
  if (fields.has('user')) payload.user = data.user;
  if (fields.has('tty')) payload.tty = data.tty;
  if (fields.has('openStdin')) payload.openStdin = data.openStdin;
  if (fields.has('stopSignal')) payload.stopSignal = data.stopSignal;
  if (fields.has('healthcheck')) {
    if (data.healthcheck.enabled) {
      payload.healthcheck = {
        test: ['CMD-SHELL', data.healthcheck.command],
        interval: data.healthcheck.interval * 1e9,
        timeout: data.healthcheck.timeout * 1e9,
        retries: data.healthcheck.retries,
        startPeriod: data.healthcheck.startPeriod * 1e9,
      };
    } else {
      payload.healthcheck = { test: ['NONE'] };
    }
  }
  if (fields.has('exposedPorts')) {
    const ports: Record<string, object> = {};
    for (const p of data.exposedPorts) ports[p] = {};
    payload.exposedPorts = ports;
  }
  if (fields.has('portMappings')) {
    payload.portBindings = portMappingsToBindings(data.portMappings);
    // Also update exposed ports to match
    const ports: Record<string, object> = {};
    for (const m of data.portMappings) ports[`${m.containerPort}/${m.protocol}`] = {};
    payload.exposedPorts = ports;
  }
  if (fields.has('networkMode')) payload.networkMode = data.networkMode;
  if (fields.has('dns')) payload.dns = data.dns;
  if (fields.has('dnsSearch')) payload.dnsSearch = data.dnsSearch;
  if (fields.has('dnsOptions')) payload.dnsOptions = data.dnsOptions;
  if (fields.has('extraHosts')) payload.extraHosts = data.extraHosts;
  if (fields.has('privileged')) payload.privileged = data.privileged;
  if (fields.has('readonlyRootfs')) payload.readonlyRootfs = data.readonlyRootfs;
  if (fields.has('capAdd')) payload.capAdd = data.capAdd;
  if (fields.has('capDrop')) payload.capDrop = data.capDrop;
  if (fields.has('securityOpt')) payload.securityOpt = data.securityOpt;
  if (fields.has('shmSize')) payload.shmSize = data.shmSize;
  if (fields.has('pidMode')) payload.pidMode = data.pidMode;
  if (fields.has('init')) payload.init = data.init;
  if (fields.has('autoRemove')) payload.autoRemove = data.autoRemove;
  if (fields.has('oomKillDisable')) payload.oomKillDisable = data.oomKillDisable;
  if (fields.has('pidsLimit')) payload.pidsLimit = data.pidsLimit;
  if (
    fields.has('gpuEnabled') ||
    fields.has('gpuDriver') ||
    fields.has('gpuCount') ||
    fields.has('gpuDeviceIds') ||
    fields.has('gpuCapabilities')
  ) {
    payload.deviceRequests = buildDeviceRequestsPayload(data);
  }
  if (fields.has('logDriver')) payload.logDriver = data.logDriver;
  if (fields.has('logOptions')) payload.logOptions = data.logOptions;

  // Also include resource overrides for config changes
  if (fields.has('memory')) payload.memory = data.memory;
  if (fields.has('nanoCpus')) payload.nanoCPUs = data.nanoCpus;

  return payload;
}
