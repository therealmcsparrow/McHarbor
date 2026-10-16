// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { Dispatch, SetStateAction } from 'react';
import type { SelectOption } from '@resources/components/ui/Select';
import type { NetworkChoice, PortBinding, VolumeMount } from '../hooks/useCreateContainer';

export const ENTRYPOINT_EXAMPLE = '/entrypoint.sh';
export const WORKING_DIR_EXAMPLE = '/app';

export type CreateContainerTab = 'form' | 'compose' | 'run';

export type EnvRow = { key: string; value: string };

export const EMPTY_PORT: PortBinding = { host: '', container: '', protocol: 'tcp' };
export const EMPTY_VOLUME: VolumeMount = { host: '', container: '', mode: 'rw' };
export const DEFAULT_NETWORK: NetworkChoice = { kind: 'default', driver: 'bridge' };

export const RESTART_OPTIONS: SelectOption[] = [
  { value: 'no', label: 'No' },
  { value: 'always', label: 'Always' },
  { value: 'on-failure', label: 'On failure' },
  { value: 'unless-stopped', label: 'Unless stopped' },
];

export function updateList<T extends object>(setter: Dispatch<SetStateAction<T[]>>, id: string, value: T) {
  setter((prev) =>
    prev.map((item) => {
      const rowId = (item as { __id?: string }).__id;
      return rowId === id ? value : item;
    }),
  );
}

export function splitEnv(line: string): EnvRow {
  const idx = line.indexOf('=');
  if (idx < 0) return { key: line, value: '' };
  return { key: line.slice(0, idx), value: line.slice(idx + 1) };
}
