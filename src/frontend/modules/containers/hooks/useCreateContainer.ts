// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { DeviceMappingEntry } from '../types/edit-form';

export type PortBinding = {
  host: string;
  container: string;
  protocol: 'tcp' | 'udp';
};

export type VolumeMount = {
  host: string;
  container: string;
  mode: 'rw' | 'ro';
};

export type RestartPolicy = 'no' | 'always' | 'on-failure' | 'unless-stopped';

/**
 * GPU access, expressed the way the container edit form already does it
 * (see `EditFormData.gpu*`) so both surfaces produce the same
 * `HostConfig.DeviceRequests` entry and a container created with a GPU
 * reads back identically in the Resources tab.
 *
 * `driver` is free text with the host's runtimes offered as suggestions:
 * "nvidia" covers the overwhelming majority, but a host can register any
 * device plugin name and hard-coding a list would lock those out.
 */
export type GpuRequest = {
  enabled: boolean;
  driver: string;
  /** Ignored when deviceIds is non-empty — see buildRequestBody. */
  count: number;
  deviceIds: string[];
  capabilities: string[];
};

export const EMPTY_GPU_REQUEST: GpuRequest = {
  enabled: false,
  driver: 'nvidia',
  count: 1,
  deviceIds: [],
  capabilities: ['gpu'],
};

export type NetworkChoice =
  | { kind: 'default'; driver: 'bridge' | 'host' | 'none' }
  | { kind: 'existing'; name: string }
  // 'custom' is the "Create new" flow: the user is currently filling out
  // the inline form, the network hasn't been created yet. We stash the
  // desired driver here so the parser/handler can use it if the user
  // submits without picking anything else.
  | { kind: 'custom'; name: string; driver: string }
  // 'new' is set right after a network is successfully created and the
  // payload needs to attach by name.
  | { kind: 'new'; name: string; driver: string };

export type CreateContainerPayload = {
  name: string;
  image: string;
  cmd?: string[];
  entrypoint?: string[];
  env?: string[];
  labels?: Record<string, string>;
  ports?: PortBinding[];
  volumes?: VolumeMount[];
  network?: NetworkChoice;
  restart?: RestartPolicy;
  workingDir?: string;
  user?: string;
  hostname?: string;
  tty?: boolean;
  openStdin?: boolean;
  autoRemove?: boolean;
  gpu?: GpuRequest;
  devices?: DeviceMappingEntry[];
  // Deliver any ${secret:NAME} in env as a file under /run/secrets instead of
  // putting the resolved value in the container's environment, where
  // `docker inspect` would show it. Opt-in: it only works for images that
  // read the matching *_FILE variable.
  secretFileMode?: boolean;
};

export type CreateContainerResponse = {
  id: string;
  warnings?: string[];
};

export function useCreateContainer() {
  const envId = useEnvironmentStore((s) => s.currentId);
  const { t } = useTranslation('containers');

  return useMutation<CreateContainerResponse, Error, CreateContainerPayload>({
    mutationFn: async (payload) => {
      const url = `/containers${envId ? `?env=${envId}` : ''}`;
      const body = buildRequestBody(payload);
      const res = await api.post<CreateContainerResponse>(url, body);
      return assertSuccess(res);
    },
    onSuccess: () => {
      toast.success(t('toast.created'));
    },
    meta: {
      success: t('toast.created'),
      error: t('create.errors.createFailed'),
    },
  });
}

// buildRequestBody translates the friendly CreateContainerPayload into the
// wire format expected by POST /containers on the backend
// (see src/backend/modules/containers/model.go::CreateRequest).
function buildRequestBody(payload: CreateContainerPayload) {
  const env = (payload.env ?? []).filter((line) => line.trim().length > 0);
  const cmd = payload.cmd && payload.cmd.length > 0 ? payload.cmd : undefined;
  const entrypoint = payload.entrypoint && payload.entrypoint.length > 0 ? payload.entrypoint : undefined;
  const labels = payload.labels && Object.keys(payload.labels).length > 0 ? payload.labels : undefined;

  const hostConfig: Record<string, unknown> = {};
  const portBindings: Record<string, Array<{ HostIp: string; HostPort: string }>> = {};
  const exposedPorts: Record<string, object> = {};
  for (const port of payload.ports ?? []) {
    const hostPortNum = Number(port.host);
    const containerPortNum = Number(port.container);
    if (!Number.isFinite(containerPortNum) || containerPortNum <= 0) continue;
    const portSpec = `${containerPortNum}/${port.protocol}`;
    exposedPorts[portSpec] = {};
    if (Number.isFinite(hostPortNum) && hostPortNum > 0) {
      portBindings[portSpec] = [{ HostIp: '0.0.0.0', HostPort: String(hostPortNum) }];
    }
  }
  if (Object.keys(portBindings).length > 0) hostConfig.PortBindings = portBindings;

  if (payload.restart && payload.restart !== 'no') {
    hostConfig.RestartPolicy = { Name: payload.restart, MaximumRetryCount: 0 };
  }

  if (payload.autoRemove) {
    hostConfig.AutoRemove = true;
  }

  // DeviceRequests is how Docker expresses `--gpus`. Count and DeviceIDs
  // are mutually exclusive in the API: naming specific devices requires
  // Count 0, and sending both makes the daemon reject the create. The
  // edit form applies the same rule, so a container round-trips between
  // the two surfaces unchanged.
  if (payload.gpu?.enabled) {
    const capabilities = payload.gpu.capabilities.filter(Boolean);
    const deviceIds = payload.gpu.deviceIds.filter(Boolean);
    hostConfig.DeviceRequests = [
      {
        Driver: payload.gpu.driver.trim() || 'nvidia',
        Count: deviceIds.length > 0 ? 0 : payload.gpu.count,
        DeviceIDs: deviceIds.length > 0 ? deviceIds : undefined,
        Capabilities: [capabilities.length > 0 ? capabilities : ['gpu']],
      },
    ];
  }

  const devices = (payload.devices ?? [])
    .map((device) => ({
      PathOnHost: device.pathOnHost.trim(),
      PathInContainer: device.pathInContainer.trim() || device.pathOnHost.trim(),
      CgroupPermissions: device.cgroupPermissions.trim() || 'rwm',
    }))
    .filter((device) => device.PathOnHost);
  if (devices.length > 0) hostConfig.Devices = devices;

  const binds: string[] = [];
  for (const vol of payload.volumes ?? []) {
    if (!vol.host || !vol.container) continue;
    const suffix = vol.mode === 'ro' ? ':ro' : '';
    binds.push(`${vol.host}:${vol.container}${suffix}`);
  }
  if (binds.length > 0) hostConfig.Binds = binds;

  const network = payload.network ?? ({ kind: 'default', driver: 'bridge' } as const);

  // Map the friendly network choice into Docker's two-level model:
  //   hostConfig.NetworkMode    → special built-in network modes ("host", "none", or the name of a custom network)
  //   networkConfig.EndpointsConfig → explicit endpoint binding to a user-defined network
  let endpointsConfig: Record<string, unknown> | undefined;
  if (network.kind === 'default' && network.driver === 'host') {
    hostConfig.NetworkMode = 'host';
  } else if (network.kind === 'default' && network.driver === 'none') {
    hostConfig.NetworkMode = 'none';
  } else if (network.kind === 'default') {
    hostConfig.NetworkMode = network.driver;
  } else {
    // existing or custom — attach by name
    hostConfig.NetworkMode = network.name;
    endpointsConfig = { [network.name]: {} };
  }

  const body: Record<string, unknown> = {
    image: payload.image,
    name: payload.name || undefined,
  };
  if (env.length > 0) body.env = env;
  if (cmd) body.cmd = cmd;
  if (entrypoint) body.entrypoint = entrypoint;
  if (labels) body.labels = labels;
  if (Object.keys(exposedPorts).length > 0) body.exposedPorts = exposedPorts;
  if (payload.workingDir) body.workingDir = payload.workingDir;
  if (payload.user) body.user = payload.user;
  if (payload.hostname) body.hostname = payload.hostname;
  if (payload.tty) body.tty = true;
  if (payload.openStdin) body.openStdin = true;
  if (Object.keys(hostConfig).length > 0) body.hostConfig = hostConfig;
  if (endpointsConfig) body.networkConfig = { EndpointsConfig: endpointsConfig };
  if (payload.secretFileMode) body.secretFileMode = true;

  return body;
}

export function useStartContainer() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useMutation<unknown, Error, string>({
    mutationFn: (id) => api.post(`/containers/${id}/start${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
  });
}
