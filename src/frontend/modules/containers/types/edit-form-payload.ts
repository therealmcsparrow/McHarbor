// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeClassification, EditFormData, PortMappingEntry } from './edit-form';
import { mountsToPayload } from './mounts-edit';

type DeviceRequestPayload = {
  Driver: string;
  Count: number;
  DeviceIDs?: string[];
  Capabilities: string[][];
};

export function buildUpdatePayload(data: EditFormData, changes: ChangeClassification): Record<string, unknown> {
  const payload: Record<string, unknown> = {};

  for (const field of changes.changedResourceFields) {
    switch (field) {
      case 'memory':
        payload.memory = data.memory;
        break;
      case 'memorySwap':
        payload.memorySwap = data.memorySwap;
        break;
      case 'memoryReservation':
        payload.memoryReservation = data.memoryReservation;
        break;
      case 'nanoCpus':
        payload.nanoCPUs = data.nanoCpus;
        break;
      case 'cpuShares':
        payload.cpuShares = data.cpuShares;
        break;
      case 'cpuPeriod':
        payload.cpuPeriod = data.cpuPeriod;
        break;
      case 'cpuQuota':
        payload.cpuQuota = data.cpuQuota;
        break;
      case 'cpusetCpus':
        payload.cpusetCpus = data.cpusetCpus;
        break;
      case 'cpusetMems':
        payload.cpusetMems = data.cpusetMems;
        break;
      case 'blkioWeight':
        payload.blkioWeight = data.blkioWeight;
        break;
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

function portMappingsToBindings(
  mappings: PortMappingEntry[],
): Record<string, Array<{ HostIp: string; HostPort: string }>> {
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

function buildDevicesPayload(data: EditFormData): Array<{
  PathOnHost: string;
  PathInContainer: string;
  CgroupPermissions: string;
}> {
  return data.devices
    .map((device) => ({
      PathOnHost: device.pathOnHost.trim(),
      PathInContainer: device.pathInContainer.trim() || device.pathOnHost.trim(),
      CgroupPermissions: device.cgroupPermissions.trim() || 'rwm',
    }))
    .filter((device) => device.PathOnHost && device.PathInContainer);
}

export function buildRecreatePayload(data: EditFormData, changes: ChangeClassification): Record<string, unknown> {
  const payload: Record<string, unknown> = buildUpdatePayload(data, changes);
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
    const ports: Record<string, object> = Object.fromEntries(data.exposedPorts.map((port) => [port, {}]));
    for (const mapping of data.portMappings) {
      ports[`${mapping.containerPort}/${mapping.protocol}`] = {};
    }
    payload.exposedPorts = ports;
  }
  if (fields.has('networkMode')) payload.networkMode = data.networkMode;
  if (fields.has('dns')) payload.dns = data.dns;
  if (fields.has('dnsSearch')) payload.dnsSearch = data.dnsSearch;
  if (fields.has('dnsOptions')) payload.dnsOptions = data.dnsOptions;
  if (fields.has('extraHosts')) payload.extraHosts = data.extraHosts;
  if (fields.has('mounts')) payload.mounts = mountsToPayload(data.mounts);
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
  if (fields.has('devices')) payload.devices = buildDevicesPayload(data);
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

  return payload;
}
