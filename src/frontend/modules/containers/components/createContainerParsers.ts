// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { CreateContainerPayload, VolumeMount } from '../hooks/useCreateContainer';
import { dockerRunTokenize, parsePort, restartFromFlag, splitOnce } from './createContainerTokenize';

export type DockerRunParseResult = {
  payload: Omit<CreateContainerPayload, 'image'> & { image: string };
  warnings: string[];
};

// parseDockerRun translates a `docker run [flags] image [command] [args]`
// command line into a CreateContainerPayload that the form can submit.
// Anything we can't model (--cap-add, healthcheck, ulimits, etc.) is
// surfaced as a warning so the user knows the value was dropped.
export function parseDockerRun(input: string): DockerRunParseResult {
  const warnings: string[] = [];
  const tokens = dockerRunTokenize(input.trim());

  // Strip leading "docker run" if present (case insensitive).
  let cursor = 0;
  if (tokens[cursor]?.toLowerCase() === 'docker') cursor += 1;
  if (tokens[cursor]?.toLowerCase() === 'run') cursor += 1;

  const payload: CreateContainerPayload = {
    name: '',
    image: '',
    ports: [],
    volumes: [],
    env: [],
    network: { kind: 'default', driver: 'bridge' },
    restart: 'no',
  };

  const knownFlags = new Set([
    '-d',
    '--detach',
    '--rm',
    '-i',
    '-t',
    '-it',
    '--interactive',
    '--tty',
    '--name',
    '-p',
    '--publish',
    '-e',
    '--env',
    '-v',
    '--volume',
    '--mount',
    '--restart',
    '--network',
    '--net',
    '--hostname',
    '-h',
    '--user',
    '-u',
    '--workdir',
    '-w',
    '--entrypoint',
    '--env-file',
    '--label',
    '-l',
    '--expose',
    '--add-host',
    '--dns',
    '--dns-search',
    '--dns-opt',
    '--domainname',
    '--platform',
    '--pid',
    '--privileged',
    '--read-only',
    '--stop-signal',
    '--stop-timeout',
    '--cap-add',
    '--cap-drop',
    '--health-cmd',
    '--health-interval',
    '--health-retries',
    '--health-start-period',
    '--health-timeout',
    '--shm-size',
    '--sysctl',
    '--ulimit',
    '--device',
    '--tmpfs',
  ]);

  // Pre-split any `--flag=value` tokens so the existing flag handlers
  // (which look at `tokens[cursor + 1]` for the value) work without
  // a per-flag special case. The token becomes `--flag`, followed by
  // a synthetic `value` token.
  for (let i = 0; i < tokens.length; i += 1) {
    const t = tokens[i];
    if (t && t.startsWith('-') && t.includes('=')) {
      const eqIdx = t.indexOf('=');
      const flag = t.slice(0, eqIdx);
      const value = t.slice(eqIdx + 1);
      tokens.splice(i, 1, flag, value);
      i += 1;
    }
  }

  while (cursor < tokens.length) {
    const tok = tokens[cursor];
    if (tok === undefined) break;
    if (tok === '-d' || tok === '--detach') {
      cursor += 1;
      continue;
    }
    if (tok === '--rm') {
      payload.autoRemove = true;
      cursor += 1;
      continue;
    }
    if (tok === '-i' || tok === '--interactive') {
      payload.openStdin = true;
      cursor += 1;
      continue;
    }
    if (tok === '-t' || tok === '--tty' || tok === '-it') {
      payload.tty = true;
      payload.openStdin = true;
      cursor += 1;
      continue;
    }
    if (tok === '--name' || tok === '-n') {
      payload.name = tokens[cursor + 1] ?? '';
      cursor += 2;
      continue;
    }
    if (tok === '-p' || tok === '--publish') {
      const value = tokens[cursor + 1] ?? '';
      const port = parsePort(value);
      if (port) (payload.ports ??= []).push(port);
      else warnings.push(`Unrecognized port mapping: ${value}`);
      cursor += 2;
      continue;
    }
    if (tok === '-e' || tok === '--env') {
      const value = tokens[cursor + 1] ?? '';
      if (value.includes('=')) (payload.env ??= []).push(value);
      else warnings.push(`Skipped -e value without '=' : ${value}`);
      cursor += 2;
      continue;
    }
    if (tok === '-v' || tok === '--volume' || tok === '--mount') {
      const value = tokens[cursor + 1] ?? '';
      const parts = splitOnce(value, ':');
      if (parts && parts[0] && parts[1]) {
        const tail = parts[1].split(':');
        const mode: VolumeMount['mode'] = tail.includes('ro') ? 'ro' : 'rw';
        (payload.volumes ??= []).push({ host: parts[0], container: tail[0] ?? '', mode });
        // Shell variables like `$(pwd)` / `$HOME` only expand in a real
        // shell. The Docker daemon receives them as literal volume names
        // and rejects with 400. Surface a heads-up so the user replaces
        // them with an absolute path (or a named volume) before submit.
        if (/\$[({]/.test(parts[0])) {
          warnings.push(
            `Volume host "${parts[0]}" contains a shell variable; the daemon receives it as a literal name. Replace with an absolute path or a named volume.`,
          );
        }
      } else {
        warnings.push(`Unrecognized volume mapping: ${value}`);
      }
      cursor += 2;
      continue;
    }
    if (tok === '--restart') {
      const value = tokens[cursor + 1] ?? '';
      const restart = restartFromFlag(value);
      if (restart) payload.restart = restart;
      else warnings.push(`Unrecognized restart policy: ${value}`);
      cursor += 2;
      continue;
    }
    if (tok === '--network' || tok === '--net') {
      const value = tokens[cursor + 1] ?? '';
      if (value === 'bridge' || value === 'host' || value === 'none') {
        payload.network = { kind: 'default', driver: value };
      } else {
        payload.network = { kind: 'existing', name: value };
      }
      cursor += 2;
      continue;
    }
    if (tok === '-h' || tok === '--hostname') {
      payload.hostname = tokens[cursor + 1] ?? '';
      cursor += 2;
      continue;
    }
    if (tok === '-u' || tok === '--user') {
      payload.user = tokens[cursor + 1] ?? '';
      cursor += 2;
      continue;
    }
    if (tok === '-w' || tok === '--workdir') {
      payload.workingDir = tokens[cursor + 1] ?? '';
      cursor += 2;
      continue;
    }
    if (tok === '--entrypoint') {
      const value = tokens[cursor + 1] ?? '';
      payload.entrypoint = value.length > 0 ? dockerRunTokenize(value) : [];
      cursor += 2;
      continue;
    }
    if (tok === '--label' || tok === '-l') {
      const value = tokens[cursor + 1] ?? '';
      const split = splitOnce(value, '=');
      if (split) {
        payload.labels = payload.labels ?? {};
        payload.labels[split[0]] = split[1];
      } else {
        warnings.push(`Skipped --label without '=' : ${value}`);
      }
      cursor += 2;
      continue;
    }
    if (tok === '--env-file') {
      warnings.push('--env-file is not supported in the form preview, add variables manually');
      cursor += 2;
      continue;
    }
    if (tok.startsWith('-')) {
      if (knownFlags.has(tok)) {
        warnings.push(`Flag ${tok} is not yet modeled in this preview`);
        cursor += 1;
        if (cursor < tokens.length && !tokens[cursor]?.startsWith('-')) cursor += 1;
      } else {
        warnings.push(`Unknown flag ${tok}`);
        cursor += 1;
      }
      continue;
    }
    // First non-flag, non-value token is the image. Everything after is command.
    payload.image = tok;
    const remaining = tokens.slice(cursor + 1);
    if (remaining.length > 0) {
      payload.cmd = remaining;
    }
    break;
  }

  if (!payload.image) {
    throw new Error('No image found in command');
  }

  return {
    payload: payload as DockerRunParseResult['payload'],
    warnings,
  };
}
