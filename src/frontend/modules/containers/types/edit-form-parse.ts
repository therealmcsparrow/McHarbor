// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData, PortMappingEntry } from './edit-form';
import { parseMounts } from './mounts-edit';

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
  const gpuCount = typeof gpuRequest?.Count === 'number' && gpuRequest.Count !== 0 ? gpuRequest.Count : -1;

  return {
    image: c.Config?.Image ?? '',

    env: c.Config?.Env ?? [],
    labels: { ...c.Config?.Labels },
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

    mounts: parseMounts(c),

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
    devices: (hc?.Devices ?? []).map((device) => ({
      pathOnHost: device.PathOnHost ?? '',
      pathInContainer: device.PathInContainer ?? device.PathOnHost ?? '',
      cgroupPermissions: device.CgroupPermissions ?? 'rwm',
    })),
    gpuEnabled: !!gpuRequest,
    gpuDriver: gpuRequest?.Driver ?? 'nvidia',
    gpuCount,
    gpuDeviceIds,
    gpuCapabilities,

    logDriver: hc?.LogConfig?.Type ?? '',
    logOptions: { ...hc?.LogConfig?.Config },
  };
}
