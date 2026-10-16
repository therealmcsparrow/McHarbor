// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CreateContainerPayload, VolumeMount } from '../hooks/useCreateContainer';
import type { DockerRunParseResult } from './createContainerParsers';
import { dockerRunTokenize, parsePort, restartFromFlag, splitOnce } from './createContainerTokenize';
import { parseSimpleYaml } from './createContainerYaml';

type ComposeService = {
  image?: string;
  container_name?: string;
  entrypoint?: string | string[];
  command?: string | string[];
  environment?: Record<string, unknown> | string[];
  ports?: Array<string | number | { published?: number; target?: number; protocol?: string }>;
  volumes?: Array<string | { source: string; target: string; read_only?: boolean }>;
  restart?: string;
  network_mode?: string;
  networks?: string[];
  tty?: boolean;
  stdin_open?: boolean;
  working_dir?: string;
  user?: string;
  hostname?: string;
  build?: unknown;
};

// parseComposeService reads a docker-compose YAML body and returns the
// first service's settings. Returns null if the body has no services.
export function parseComposeService(yaml: string): DockerRunParseResult {
  const warnings: string[] = [];
  const payload: CreateContainerPayload = {
    name: '',
    image: '',
    ports: [],
    volumes: [],
    env: [],
    network: { kind: 'default', driver: 'bridge' },
    restart: 'no',
  };

  const doc = parseSimpleYaml(yaml);
  if (!doc || typeof doc !== 'object') {
    throw new Error('Compose YAML must contain a top-level services: section');
  }
  const services = (doc as { services?: Record<string, unknown> }).services;
  if (!services || typeof services !== 'object') {
    throw new Error('Compose YAML must contain a top-level services: section');
  }
  const serviceNames = Object.keys(services);
  if (serviceNames.length === 0) {
    throw new Error('Compose YAML has no services');
  }
  if (serviceNames.length > 1) {
    warnings.push(`Only the first service ("${serviceNames[0]}") is used; ignoring ${serviceNames.length - 1} others`);
  }
  const service = services[serviceNames[0] ?? ''] as ComposeService | undefined;
  if (!service || typeof service !== 'object') {
    throw new Error('Compose service block is not an object');
  }

  if (typeof service.image === 'string') {
    payload.image = service.image;
  } else if (service.build !== undefined) {
    throw new Error('Compose services that use `build:` are not supported in this preview');
  } else {
    throw new Error('Compose service has no image');
  }

  if (typeof service.container_name === 'string') {
    payload.name = service.container_name;
  }

  if (typeof service.entrypoint === 'string') {
    payload.entrypoint = dockerRunTokenize(service.entrypoint);
  } else if (Array.isArray(service.entrypoint)) {
    payload.entrypoint = service.entrypoint.map(String);
  }

  if (typeof service.command === 'string') {
    payload.cmd = dockerRunTokenize(service.command);
  } else if (Array.isArray(service.command)) {
    payload.cmd = service.command.map(String);
  }

  if (service.environment) {
    const env = service.environment;
    if (Array.isArray(env)) {
      for (const item of env) {
        if (typeof item === 'string' && item.includes('=')) (payload.env ??= []).push(item);
      }
    } else if (typeof env === 'object') {
      for (const [key, value] of Object.entries(env)) {
        if (value == null) {
          (payload.env ??= []).push(key);
        } else {
          (payload.env ??= []).push(
            `${key}=${
              typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean'
                ? String(value)
                : JSON.stringify(value)
            }`,
          );
        }
      }
    }
  }

  if (service.ports && Array.isArray(service.ports)) {
    for (const raw of service.ports) {
      let value: string;
      if (typeof raw === 'string') {
        value = raw;
      } else if (typeof raw === 'number') {
        value = String(raw);
      } else if (raw && typeof raw === 'object' && 'published' in raw) {
        const obj = raw as { published?: number; target?: number; protocol?: string };
        value = `${obj.published ?? ''}:${obj.target ?? ''}${obj.protocol ? `/${obj.protocol}` : ''}`;
      } else {
        continue;
      }
      const port = parsePort(value);
      if (port) (payload.ports ??= []).push(port);
    }
  }

  if (service.volumes && Array.isArray(service.volumes)) {
    for (const raw of service.volumes) {
      let value: string;
      if (typeof raw === 'string') {
        value = raw;
      } else if (raw && typeof raw === 'object' && 'source' in raw && 'target' in raw) {
        const obj = raw as { source: string; target: string; read_only?: boolean };
        value = `${obj.source}:${obj.target}${obj.read_only ? ':ro' : ''}`;
      } else {
        continue;
      }
      const parts = splitOnce(value, ':');
      if (parts && parts[0] && parts[1]) {
        const tail = parts[1].split(':');
        const mode: VolumeMount['mode'] = tail.includes('ro') ? 'ro' : 'rw';
        (payload.volumes ??= []).push({ host: parts[0], container: tail[0] ?? '', mode });
      }
    }
  }

  if (service.restart && typeof service.restart === 'string') {
    const restart = restartFromFlag(service.restart);
    if (restart) payload.restart = restart;
  }

  if (typeof service.network_mode === 'string') {
    const v = service.network_mode;
    if (v === 'bridge' || v === 'host' || v === 'none') {
      payload.network = { kind: 'default', driver: v };
    } else {
      payload.network = { kind: 'existing', name: v };
    }
  } else if (Array.isArray(service.networks) && service.networks.length > 0) {
    const first = service.networks[0];
    if (typeof first === 'string') {
      payload.network = { kind: 'existing', name: first };
      if (service.networks.length > 1) {
        warnings.push(`Only the first network ("${first}") is used; ignoring ${service.networks.length - 1} others`);
      }
    }
  }

  if (service.tty === true) payload.tty = true;
  if (service.stdin_open === true) payload.openStdin = true;

  if (service.working_dir && typeof service.working_dir === 'string') {
    payload.workingDir = service.working_dir;
  }
  if (service.user && typeof service.user === 'string') {
    payload.user = service.user;
  }
  if (service.hostname && typeof service.hostname === 'string') {
    payload.hostname = service.hostname;
  }

  return {
    payload: payload as DockerRunParseResult['payload'],
    warnings,
  };
}
