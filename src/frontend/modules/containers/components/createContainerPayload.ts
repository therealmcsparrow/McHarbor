// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import type {
  CreateContainerPayload,
  GpuRequest,
  NetworkChoice,
  PortBinding,
  RestartPolicy,
  VolumeMount,
} from '../hooks/useCreateContainer';
import type { DeviceMappingEntry } from '../types/edit-form';
import { DEFAULT_NETWORK, type EnvRow } from './createContainerFormHelpers';
import { dockerRunTokenize } from './createContainerTokenize';

export type CreateContainerFormValues = {
  image: string;
  entryName: string;
  ports: PortBinding[];
  env: EnvRow[];
  secretFileMode: boolean;
  volumes: VolumeMount[];
  network: NetworkChoice;
  restart: RestartPolicy;
  autoRemove: boolean;
  tty: boolean;
  stdin: boolean;
  gpu: GpuRequest;
  devices: DeviceMappingEntry[];
  cmd: string;
  entrypoint: string;
  workingDir: string;
  user: string;
  hostname: string;
};

export function buildPayloadFromForm(
  form: CreateContainerFormValues,
  t: TFunction<'containers'>,
): CreateContainerPayload | { error: string } {
  if (!form.image.trim()) {
    return { error: t('create.errors.imageRequired') };
  }
  const trimmedName = form.entryName.trim();

  const portList: PortBinding[] = [];
  for (const port of form.ports) {
    if (!port.host && !port.container) continue;
    const containerNum = Number(port.container);
    if (!Number.isFinite(containerNum) || containerNum <= 0 || containerNum > 65535) {
      return { error: t('create.errors.portContainerInvalid') };
    }
    const hostNum = Number(port.host);
    if (port.host && (!Number.isFinite(hostNum) || hostNum <= 0 || hostNum > 65535)) {
      return { error: t('create.errors.portHostInvalid') };
    }
    portList.push({
      host: port.host,
      container: String(containerNum),
      protocol: port.protocol,
    });
  }

  const volumeList: VolumeMount[] = [];
  for (const vol of form.volumes) {
    if (!vol.host && !vol.container) continue;
    if (!vol.host) {
      return { error: t('create.errors.volumeHostRequired') };
    }
    if (!vol.container) {
      return { error: t('create.errors.volumeContainerRequired') };
    }
    volumeList.push({ host: vol.host, container: vol.container, mode: vol.mode });
  }

  const envList: string[] = [];
  for (const entry of form.env) {
    const key = entry.key.trim();
    if (!key && !entry.value) continue;
    if (!key) {
      return { error: t('create.errors.envInvalid') };
    }
    envList.push(`${key}=${entry.value}`);
  }

  const cmdList = form.cmd.trim().length > 0 ? dockerRunTokenize(form.cmd) : undefined;
  const entrypointList = form.entrypoint.trim().length > 0 ? dockerRunTokenize(form.entrypoint) : undefined;

  // A network still in the "Create new network" draft state would attach to a
  // bogus name, so it falls back to the bridge default.
  const safeNetwork: NetworkChoice = form.network.kind === 'custom' ? DEFAULT_NETWORK : form.network;

  return {
    name: trimmedName,
    image: form.image.trim(),
    cmd: cmdList,
    entrypoint: entrypointList,
    env: envList,
    ports: portList.length > 0 ? portList : undefined,
    volumes: volumeList.length > 0 ? volumeList : undefined,
    network: safeNetwork,
    restart: form.restart,
    autoRemove: form.autoRemove,
    tty: form.tty,
    openStdin: form.stdin,
    gpu: form.gpu.enabled ? form.gpu : undefined,
    devices: form.devices.length > 0 ? form.devices : undefined,
    workingDir: form.workingDir.trim() || undefined,
    user: form.user.trim() || undefined,
    hostname: form.hostname.trim() || undefined,
    secretFileMode: form.secretFileMode,
  };
}
